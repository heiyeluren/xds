// Copyright (c) 2022 XDS project Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// XDS Project Site: https://github.com/heiyeluren
// XDS URL: https://github.com/heiyeluren/xds
//

package xmap

import (
	"bytes"
	"errors"
	"github.com/heiyeluren/xds/xmap/entry"
	"github.com/heiyeluren/xmm"
	"log"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

// MinTransferStride is the minimum number of entries to transfer between
// 1、步长resize
// 2、sizeCtl增加cap的cas，不允许提前resize。
// 考虑 数组+ 链表方式
const MinTransferStride = 16
const maximumCapacity = 1 << 30

// KeyExists 标识key已经存在
const KeyExists = true

// KeyNotExists 标识key不存在
const KeyNotExists = false

var NotFound = errors.New("not found")
var _BucketSize = unsafe.Sizeof(Bucket{})
var _ForwardingBucketSize = unsafe.Sizeof(ForwardingBucket{})
var _NodeEntrySize = unsafe.Sizeof(entry.NodeEntry{})
var _TreeSize = unsafe.Sizeof(entry.Tree{})
var uintPtrSize = uintptr(8)

// ConcurrentRawHashMap is a concurrent hash map with a fixed number of buckets.
// 清理方式：1、del清除entry(简单)
// 		2、buckets清除旧的（resize结束，并无get读引用【引入重试来解决该问题】）
// 		3、Bucket清除
// 		4、ForwardingBucket清除
// 		5、Tree 清除（spliceEntry2时候）
type ConcurrentRawHashMap struct {
	size      uint64
	threshold uint64
	initCap   uint64
	// off-heap
	buckets *[]uintptr
	mm      xmm.XMemory
	lock    sync.RWMutex

	treeSize uint64

	// resize
	sizeCtl   int64 // -1 正在扩容
	reSizeGen uint64

	nextBuckets   *[]uintptr
	transferIndex uint64

	destroyed uint32 // 0:未销毁   1:已经销毁

	destroyLock sync.RWMutex
}

// Bucket is a hash bucket.
type Bucket struct {
	forwarding bool // 已经迁移完成
	rwLock     sync.RWMutex
	index      uint64
	newBuckets *[]uintptr
	Head       *entry.NodeEntry
	Tree       *entry.Tree
	isTree     bool
	size       uint64
}

// ForwardingBucket is a hash bucket that has been forwarded to a new table.
type ForwardingBucket struct {
	forwarding bool // 已经迁移完成
	rwLock     sync.RWMutex
	index      uint64
	newBuckets *[]uintptr
}

// Snapshot 利用快照比对产生
type Snapshot struct {
	buckets     *[]uintptr
	sizeCtl     int64 // -1 正在扩容
	nextBuckets *[]uintptr
}

// NewDefaultConcurrentRawHashMap returns a new ConcurrentRawHashMap with the default
// mm: xmm
func NewDefaultConcurrentRawHashMap(mm xmm.XMemory) (*ConcurrentRawHashMap, error) {
	return NewConcurrentRawHashMap(mm, 16, 0.75, 8)
}

// NewConcurrentRawHashMap will create a new ConcurrentRawHashMap with the given
// mm: xmm 内存对象
// cap:初始化bucket长度 （可以理解为 map 元素预计最大个数~，如果知道这个值可以提前传递）
// fact:负责因子，当存放的元素超过该百分比，就会触发扩容。
// treeSize：bucket中的链表长度达到该值后，会转换为红黑树。
func NewConcurrentRawHashMap(mm xmm.XMemory, cap uintptr, fact float64, treeSize uint64) (*ConcurrentRawHashMap, error) {
	if cap < 1 {
		return nil, errors.New("cap < 1")
	}
	var alignCap uintptr
	for i := 1; i < 64 && cap > alignCap; i++ {
		alignCap = 1 << uint(i)
	}
	cap = alignCap
	bucketsPtr, err := mm.AllocSlice(uintPtrSize, cap, cap)
	if err != nil {
		return nil, err
	}
	buckets := (*[]uintptr)(bucketsPtr)
	return &ConcurrentRawHashMap{buckets: buckets, initCap: uint64(cap), threshold: uint64(float64(cap) * fact),
		mm: mm, treeSize: treeSize}, nil
}

func (chm *ConcurrentRawHashMap) getBucket(h uint64, tab *[]uintptr) *Bucket {
	mask := uint64(cap(*tab) - 1)
	idx := h & mask
	_, _, bucket := chm.tabAt(tab, idx)
	if bucket != nil && bucket.forwarding && chm.transferIndex >= 0 {
		return chm.getBucket(h, bucket.newBuckets)
	}
	return bucket
}

// Get Fetch key from hashmap
func (chm *ConcurrentRawHashMap) Get(key []byte) (val []byte, keyExists bool, err error) {
	h := BKDRHashWithSpread(key)
	bucket := chm.getBucket(h, chm.buckets)
	if bucket == nil {
		return nil, KeyNotExists, NotFound
	}
	if bucket.isTree {
		exist, value := bucket.Tree.Get(key)
		if !exist {
			return nil, KeyNotExists, NotFound
		}
		return value, KeyExists, nil
	}
	keySize := len(key)
	for cNode := bucket.Head; cNode != nil; cNode = cNode.Next {
		if keySize == len(cNode.Key) && bytes.Compare(key, cNode.Key) == 0 {
			return cNode.Value, KeyExists, nil
		}
	}
	return nil, KeyNotExists, NotFound
}
func (chm *ConcurrentRawHashMap) initForwardingEntries(newBuckets *[]uintptr, index uint64) (*ForwardingBucket, error) {
	entriesPtr, err := chm.mm.Alloc(_ForwardingBucketSize)
	if err != nil {
		return nil, err
	}
	entries := (*ForwardingBucket)(entriesPtr)
	chm.assignmentForwardingEntries(newBuckets, entries, index)
	return entries, nil
}

func (chm *ConcurrentRawHashMap) assignmentForwardingEntries(newBuckets *[]uintptr, entries *ForwardingBucket, index uint64) {
	entries.newBuckets = newBuckets
	entries.index = index
	entries.forwarding = true
}

func (chm *ConcurrentRawHashMap) initEntries(entry *entry.NodeEntry, idx uint64) (*Bucket, error) {
	ptr, err := chm.mm.Alloc(_BucketSize)
	if err != nil {
		return nil, err
	}
	entries := (*Bucket)(ptr)
	chm.assignmentEntries(entries, entry, idx)
	return entries, nil
}

func (chm *ConcurrentRawHashMap) assignmentEntries(entries *Bucket, entry *entry.NodeEntry, index uint64) {
	entries.index = index
	entries.Head = entry
}

func (chm *ConcurrentRawHashMap) getStride(length uint64) (stride uint64) {
	cpuNum := uint64(runtime.NumCPU())
	if cpuNum > 1 {
		stride = (length >> 3) / cpuNum
	} else {
		stride = length
	}
	if stride < MinTransferStride {
		stride = MinTransferStride
	}
	return stride
}

func (chm *ConcurrentRawHashMap) resizeStamp(length uint64) (stride int64) {
	return -10000 - int64(length)
}

func (chm *ConcurrentRawHashMap) helpTransform(entry *entry.NodeEntry, bucket *Bucket, tab *[]uintptr) (currentBucket *Bucket, init bool, currentTab *[]uintptr, err error) {
	if bucket != nil && bucket.forwarding {
		if err := chm.reHashSize(tab); err != nil {
			return nil, init, nil, err
		}
		tabPtr := bucket.newBuckets
		bucket, swapped, err := chm.getAndInitBucket(entry, tabPtr)
		if err != nil {
			return nil, init, nil, err
		}
		if swapped {
			return nil, true, nil, err
		}
		return bucket, init, tabPtr, nil
	}
	return bucket, init, tab, nil
}

func (chm *ConcurrentRawHashMap) indexAndInitBucket(entry *entry.NodeEntry) (entries *Bucket, init bool, tabPtr *[]uintptr, err error) {
	tabPtr = chm.buckets
	bucket, swapped, err := chm.getAndInitBucket(entry, tabPtr)
	if err != nil {
		return nil, init, nil, err
	}
	if swapped {
		return bucket, true, tabPtr, nil
	}
	if bucket != nil && bucket.forwarding && chm.transferIndex >= 0 {
		bucket, swapped, tabPtr, err = chm.helpTransform(entry, bucket, tabPtr)
		if err != nil {
			return nil, init, nil, err
		}
		if swapped {
			return bucket, true, tabPtr, nil
		}
	}
	return bucket, init, tabPtr, nil
}

func (chm *ConcurrentRawHashMap) index(h uint64, length int) uint64 {
	idx := h & uint64(length-1)
	return idx
}

func (chm *ConcurrentRawHashMap) tabAt(buckets *[]uintptr, idx uint64) (*uintptr, uintptr, *Bucket) {
	addr := &((*buckets)[idx])
	ptr := atomic.LoadUintptr(addr)
	if ptr == 0 {
		return addr, ptr, nil
	}
	bucket := (*Bucket)(unsafe.Pointer(ptr))
	return addr, ptr, bucket
}

// cas 设置bucket
func (chm *ConcurrentRawHashMap) getAndInitBucket(entry *entry.NodeEntry, tabPtr *[]uintptr) (bucket *Bucket, swapped bool, err error) {
	h := entry.Hash
	idx := chm.index(h, cap(*tabPtr))
	// retry := 10改小后，出现该问题。
	addr, _, bucket := chm.tabAt(tabPtr, idx)
	if bucket != nil {
		return bucket, false, nil
	}
	entity, err := chm.initEntries(entry, idx)
	if err != nil {
		return nil, false, err
	}
	ptr := uintptr(unsafe.Pointer(entity))
	atomic.CompareAndSwapUintptr(addr, 0, ptr)
	bucket = (*Bucket)(unsafe.Pointer(atomic.LoadUintptr(addr)))
	return bucket, swapped, nil
}

func (chm *ConcurrentRawHashMap) increaseSize() (newSize uint64) {
	retry := 30
	var swapped bool
	for !swapped && retry > 0 {
		retry--
		oldVal := atomic.LoadUint64(&chm.size)
		newVal := oldVal + 1
		swapped = atomic.CompareAndSwapUint64(&chm.size, oldVal, newVal)
		if swapped {
			return newVal
		}
	}
	chm.lock.Lock()
	defer chm.lock.Unlock()
	size := chm.size + 1
	chm.size = size
	return size
}

func (chm *ConcurrentRawHashMap) createEntry(key []byte, val []byte, hash uint64) (*entry.NodeEntry, error) {
	entryPtr, err := chm.mm.Alloc(_NodeEntrySize)
	if err != nil {
		return nil, err
	}
	node := (*entry.NodeEntry)(entryPtr)
	keyPtr, valPtr, err := chm.mm.Copy2(key, val)
	if err != nil {
		return nil, err
	}
	chm.entryAssignment(keyPtr, valPtr, hash, node)
	return node, nil
}

func (chm *ConcurrentRawHashMap) entryAssignment(keyPtr []byte, valPtr []byte, hash uint64, entry *entry.NodeEntry) {
	entry.Key = keyPtr
	entry.Value = valPtr
	entry.Hash = hash
}

func (chm *ConcurrentRawHashMap) entryAssignmentCpy(keyPtr []byte, valPtr []byte, hash uint64, nodePtr unsafe.Pointer) error {
	source := entry.NodeEntry{Value: keyPtr, Key: valPtr, Hash: hash}
	offset := 40 // unsafe.Offsetof(source.Next)  //40
	srcData := (*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data: uintptr(unsafe.Pointer(&source)), Len: offset, Cap: offset}))
	dstData := (*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data: uintptr(nodePtr), Len: offset, Cap: offset}))
	if offset != copy(*dstData, *srcData) {
		return errors.New("incorrect copy length") // 拷贝长度不正确
	}
	return nil
}

func (chm *ConcurrentRawHashMap) putVal(key []byte, val []byte, h uint64) (*[]uintptr, error) {
	node, err := chm.createEntry(key, val, h)
	if err != nil {
		return nil, err
	}
	loop := true
	var tabPtr *[]uintptr
	for loop {
		bucket, init, newTabPtr, err := chm.indexAndInitBucket(node)
		if err != nil {
			return nil, err
		}
		tabPtr = newTabPtr
		if init {
			break
		}
		if loop, err = chm.PutBucketValue(bucket, node, tabPtr); err != nil {
			return nil, err
		}
	}
	return tabPtr, nil
}

// Put 将键值对添加到map中
func (chm *ConcurrentRawHashMap) Put(key []byte, val []byte) error {
	h := BKDRHashWithSpread(key)
	tabPtr, err := chm.putVal(key, val, h)
	if err != nil {
		return err
	}
	size := chm.increaseSize()
	threshold := atomic.LoadUint64(&chm.threshold)
	if size >= threshold && size < maximumCapacity {
		return chm.reHashSize(tabPtr)
	}
	return nil
}

// Del 删除键值对
func (chm *ConcurrentRawHashMap) Del(key []byte) error {
	h := BKDRHashWithSpread(key)
	return chm.delVal(key, h)
}

func (chm *ConcurrentRawHashMap) delVal(key []byte, h uint64) error {
	loop := true
	var tabPtr *[]uintptr
	tabPtr = chm.buckets
	for loop {
		idx := chm.index(h, cap(*tabPtr))
		_, _, bucket := chm.tabAt(tabPtr, idx)
		if bucket == nil {
			return nil
		}
		if bucket != nil && bucket.forwarding {
			if err := chm.reHashSize(tabPtr); err != nil {
				return err
			}
			tabPtr = bucket.newBuckets
			idx = chm.index(h, cap(*tabPtr))
			_, _, bucket = chm.tabAt(tabPtr, idx)
			continue
		}
		var removeNode *entry.NodeEntry
		func() {
			// 删除bucket中的数目
			bucket.rwLock.Lock()
			defer bucket.rwLock.Unlock()
			_, _, newBucket := chm.tabAt(tabPtr, chm.index(h, cap(*tabPtr)))
			if newBucket != bucket || (bucket != nil && bucket.forwarding) {
				return
			}
			if bucket.isTree {
				if node := bucket.Tree.Delete(key); node != nil {
					removeNode = node
				}
			} else {
				keySize := len(key)
				var pre *entry.NodeEntry
				for cNode := bucket.Head; cNode != nil; cNode = cNode.Next {
					if keySize == len(cNode.Key) && bytes.Compare(key, cNode.Key) == 0 {
						removeNode = cNode
						if pre == nil {
							bucket.Head = cNode.Next
						} else {
							pre.Next = cNode.Next
						}
						break
					}
					pre = cNode
				}
			}
			loop = false
			return
		}()
		if err := chm.freeEntry(removeNode); err != nil {
			return err
		}
	}
	return nil
}

// freeEntry todo 异步free
func (chm *ConcurrentRawHashMap) freeEntry(removeNode *entry.NodeEntry) error {
	if removeNode == nil {
		return nil
	}
	if err := chm.mm.Free(uintptr(unsafe.Pointer(removeNode))); err != nil {
		return err
	}
	keySlice, valSlice := (*reflect.SliceHeader)(unsafe.Pointer(&removeNode.Key)), (*reflect.SliceHeader)(unsafe.Pointer(&removeNode.Value))
	if err := chm.mm.Free(keySlice.Data); err != nil {
		return err
	}
	if err := chm.mm.Free(valSlice.Data); err != nil {
		return err
	}
	return nil
}

func (chm *ConcurrentRawHashMap) growTree(bucket *Bucket) error {
	if bucket.isTree || bucket.size < chm.treeSize {
		return nil
	}
	treePtr, err := chm.mm.Alloc(_TreeSize)
	if err != nil {
		return err
	}
	bucket.Tree = (*entry.Tree)(treePtr)
	bucket.Tree.SetComparator(entry.BytesAscSort)
	for node := bucket.Head; node != nil; {
		next := node.Next
		if err := bucket.Tree.Put(node); err != nil {
			return err
		}
		node = next
	}
	bucket.Head = nil
	bucket.isTree = true
	return nil
}

// PutBucketValue table的可见性问题，并发问题。
func (chm *ConcurrentRawHashMap) PutBucketValue(bucket *Bucket, node *entry.NodeEntry, tab *[]uintptr) (loop bool, err error) {
	bucket.rwLock.Lock()
	defer bucket.rwLock.Unlock()
	idx := chm.index(node.Hash, cap(*tab))
	_, _, newBucket := chm.tabAt(tab, idx)
	if newBucket != bucket || newBucket.forwarding {
		return true, nil
	}
	bucket = newBucket
	// 树化
	if err := chm.growTree(bucket); err != nil {
		return false, err
	}
	if bucket.isTree {
		if err := bucket.Tree.Put(node); err != nil {
			return false, err
		}
		return false, nil
	}
	key, val := node.Key, node.Value
	var last *entry.NodeEntry
	for node := bucket.Head; node != nil; node = node.Next {
		if len(key) == len(node.Key) && bytes.Compare(node.Key, key) == 0 {
			node.Value = val
			return false, nil
		}
		if node.Next == nil {
			last = node
		}
	}
	bucket.size += 1
	// 加入
	if last == nil {
		bucket.Head = node
		return false, nil
	}
	last.Next = node
	return false, nil
}

func (chm *ConcurrentRawHashMap) expandCap(tab *[]uintptr) (s *Snapshot, need bool) {
	old, size, transferIndex, threshold := uint64(cap(*chm.buckets)), atomic.LoadUint64(&chm.size),
		atomic.LoadUint64(&chm.transferIndex), atomic.LoadUint64(&chm.threshold)
	if size < threshold || transferIndex >= old {
		return nil, false
	}
	nextBuckets, buckets, sizeCtl := chm.nextBuckets, chm.buckets, atomic.LoadInt64(&chm.sizeCtl)
	// 正在扩容
	if sizeCtl < 0 && nextBuckets != nil && cap(*nextBuckets) == int(old)*2 {
		if sizeCtl >= 0 {
			return nil, false
		}
		// 当前扩容状态正确，开始扩容
		if unsafe.Pointer(tab) == unsafe.Pointer(buckets) && nextBuckets != nil {
			if atomic.CompareAndSwapInt64(&chm.sizeCtl, sizeCtl, sizeCtl+1) {
				return &Snapshot{buckets: buckets, sizeCtl: sizeCtl, nextBuckets: nextBuckets}, true
			}
		}
		return nil, false
	}
	// 未开始扩容的判断
	if sizeCtl >= 0 && (old<<1) > uint64(sizeCtl) && unsafe.Pointer(tab) == unsafe.Pointer(buckets) {
		newSizeCtl := chm.resizeStamp(old) + 2
		swapped := atomic.CompareAndSwapInt64(&chm.sizeCtl, sizeCtl, newSizeCtl)
		// 开始扩容
		if swapped {
			newCap := old << 1
			bucketsPtr, err := chm.mm.AllocSlice(uintPtrSize, uintptr(newCap), uintptr(newCap))
			if err != nil {
				log.Printf("ConcurrentRawHashMap chm.mm.Alloc newCap:%d  err:%s \n", newCap, err)
				return nil, false
			}
			nextBuckets = (*[]uintptr)(bucketsPtr)
			chm.nextBuckets = nextBuckets
			chm.transferIndex = 0
			return &Snapshot{buckets: buckets, sizeCtl: sizeCtl, nextBuckets: nextBuckets}, true
		}
		return nil, false
	}
	return nil, false
}

func (chm *ConcurrentRawHashMap) reHashSize(tab *[]uintptr) error {
	// cas锁
	snapshot, need := chm.expandCap(tab)
	if !need {
		return nil
	}
	currentCap := uint64(cap(*snapshot.buckets))
	buckets := snapshot.nextBuckets
	// 取当前内容oldBuckets，在oldBuckets中利用bucket的lock来避免同时操作。
	err := chm.reSizeBuckets(snapshot)
	if err != nil {
		return err
	}
	for {
		sizeCtl := atomic.LoadInt64(&chm.sizeCtl)
		if sizeCtl >= 0 {
			return nil
		}
		if atomic.CompareAndSwapInt64(&chm.sizeCtl, sizeCtl, sizeCtl-1) {
			if sizeCtl != chm.resizeStamp(currentCap)+2 {
				return nil
			}
			break
		}
	}
	return func() error {
		oldBuckets, tabPtr := chm.buckets, unsafe.Pointer(&chm.buckets)
		bucketsAddr := (*unsafe.Pointer)(tabPtr)
		old := (*reflect.SliceHeader)(atomic.LoadPointer(bucketsAddr))
		if old.Cap != int(currentCap) {
			return nil
		}
		if atomic.CompareAndSwapPointer(bucketsAddr, unsafe.Pointer(old), unsafe.Pointer(buckets)) {
			// 更换内容赋值
			chm.nextBuckets = nil
			newCap := currentCap << 1
			atomic.StoreInt64(&chm.sizeCtl, int64(newCap))
			chm.threshold = chm.threshold << 1
			chm.reSizeGen += 1
			if err := chm.freeBuckets(oldBuckets); err != nil {
				log.Printf("freeBuckets err:%s\n", err)
			}
			return err
		}
		return nil
	}()
}

/*
todo 异步free  清除内存
buckets清除旧的（resize结束，并无get读引用【引入重试来解决该问题】）
3、Bucket清除
4、元素清除*/
func (chm *ConcurrentRawHashMap) freeBuckets(buckets *[]uintptr) error {
	for _, ptr := range *buckets {
		if ptr < 1 {
			continue
		}
		if err := chm.mm.Free(ptr); err != nil {
			log.Printf("freeBuckets Free element(%d) err:%s\n", ptr, err)
		}
	}
	if err := chm.mm.Free(uintptr(unsafe.Pointer(buckets))); err != nil {
		return err
	}
	return nil
}

func (chm *ConcurrentRawHashMap) increaseTransferIndex(cap uint64, stride uint64) (offset uint64, over bool) {
	for {
		transferIndex := atomic.LoadUint64(&chm.transferIndex)
		if transferIndex >= cap {
			return 0, true
		}
		swapped := atomic.CompareAndSwapUint64(&chm.transferIndex, transferIndex, transferIndex+stride)
		if swapped {
			return transferIndex, false
		}
	}
}

func (chm *ConcurrentRawHashMap) reSizeBuckets(s *Snapshot) error {
	newBuckets, oldBuckets, currentCap := s.nextBuckets, s.buckets, uint64(cap(*s.buckets))
	for {
		stride := chm.getStride(currentCap)
		offset, over := chm.increaseTransferIndex(currentCap, stride)
		if over {
			return nil
		}
		maxIndex := offset + stride
		if maxIndex > currentCap {
			maxIndex = currentCap
		}
		for index := offset; index < maxIndex; index++ {
			var err error
			loop := true
			for loop {
				addr := &((*oldBuckets)[index])
				entries := (*Bucket)(unsafe.Pointer(atomic.LoadUintptr(addr)))
				if entries == nil {
					forwardingEntries, err := chm.initForwardingEntries(newBuckets, index)
					if err != nil {
						log.Printf("ERROR reSizeBucket initForwardingEntries err:%s\n", err)
						return err
					}
					ptr := uintptr(unsafe.Pointer(forwardingEntries))
					if atomic.CompareAndSwapUintptr(addr, 0, ptr) {
						break
					} else {
						entries = (*Bucket)(unsafe.Pointer(atomic.LoadUintptr(addr)))
					}
				}
				if loop, err = chm.reSizeBucket(entries, s); err != nil {
					log.Printf("ERROR reSizeBucket  oldEntries err:%s\n", err)
				}
			}
		}
	}
}

// Chain .链接
type Chain struct {
	Head *entry.NodeEntry
	Tail *entry.NodeEntry
}

// Add 添加节点
func (c *Chain) Add(node *entry.NodeEntry) {
	if c.Head == nil {
		c.Head = node
	}
	if c.Tail != nil {
		c.Tail.Next = node
	}
	c.Tail = node
}

// GetHead 获取head节点
func (c *Chain) GetHead() *entry.NodeEntry {
	if c.Tail != nil {
		c.Tail.Next = nil
	}
	return c.Head
}

func (chm *ConcurrentRawHashMap) reSizeBucket(entries *Bucket, s *Snapshot) (loop bool, err error) {
	entries.rwLock.Lock()
	defer entries.rwLock.Unlock()
	oldBuckets, newBuckets := s.buckets, s.nextBuckets
	currentCap := uint64(cap(*s.buckets))
	tabPtr, nextTabPtr := unsafe.Pointer(s.buckets), unsafe.Pointer(s.nextBuckets)
	_, _, currentEntries := chm.tabAt(oldBuckets, entries.index)
	// 判断newBuckets为chm.newBuckets,判断newBuckets为chm.newBuckets
	if entries.forwarding || tabPtr != unsafe.Pointer(chm.buckets) ||
		nextTabPtr != unsafe.Pointer(chm.nextBuckets) || entries != currentEntries {
		return false, nil
	}
	if entries != nil && ((!entries.isTree && entries.Head == nil) || (entries.isTree && entries.Tree == nil)) {
		entries.newBuckets = newBuckets
		entries.forwarding = true
		return false, nil
	}
	mask := (currentCap << 1) - 1
	index := entries.index
	var oldIndex, newIndex = index, currentCap + index
	oldChains, newChains, err := chm.spliceEntry2(entries, mask)
	if err != nil {
		return true, err
	}
	oldBucket, newBucket, err := chm.spliceBucket2(oldChains, newChains)
	if err != nil {
		return false, err
	}
	if oldBucket != nil {
		oldBucket.index = oldIndex
		(*newBuckets)[oldIndex] = uintptr(unsafe.Pointer(oldBucket))
	}
	if newBucket != nil {
		newBucket.index = newIndex
		(*newBuckets)[newIndex] = uintptr(unsafe.Pointer(newBucket))
	}
	entries.newBuckets = newBuckets
	entries.forwarding = true
	entries.Head = nil
	return false, nil
}

// TreeSplice .
func (chm *ConcurrentRawHashMap) TreeSplice(node *entry.NodeEntry, index uint64, mask uint64, oldChains, newChains *Chain) {
	if node == nil {
		return
	}
	if node.Hash&mask == index {
		oldChains.Add(node)
	} else {
		newChains.Add(node)
	}
	chm.TreeSplice(node.Left(), index, mask, oldChains, newChains)
	chm.TreeSplice(node.Right(), index, mask, oldChains, newChains)
}

// spliceEntry2 将Entry拆分为两个列表
func (chm *ConcurrentRawHashMap) spliceEntry2(entries *Bucket, mask uint64) (old *entry.NodeEntry, new *entry.NodeEntry, err error) {
	var oldHead, oldTail, newHead, newTail *entry.NodeEntry
	index := entries.index
	if entries.isTree {
		var oldChains, newChains Chain
		chm.TreeSplice(entries.Tree.GetRoot(), index, mask, &oldChains, &newChains)
		oldHead, newHead = oldChains.GetHead(), newChains.GetHead()
		//  删除现在的tree内容
		if err := chm.mm.Free(uintptr(unsafe.Pointer(entries.Tree))); err != nil {
			return nil, nil, err
		}
	} else {
		for nodeEntry := entries.Head; nodeEntry != nil; nodeEntry = nodeEntry.Next {
			idx := nodeEntry.Hash & mask
			if idx == index {
				if oldHead == nil {
					oldHead = nodeEntry
				} else if oldTail != nil {
					oldTail.Next = nodeEntry
				}
				oldTail = nodeEntry
			} else {
				if newHead == nil {
					newHead = nodeEntry
				} else if newTail != nil {
					newTail.Next = nodeEntry
				}
				newTail = nodeEntry
			}
		}
	}
	if newTail != nil {
		newTail.Next = nil
	}
	if oldTail != nil {
		oldTail.Next = nil
	}
	return oldHead, newHead, nil
}

// 性能更好
func (chm *ConcurrentRawHashMap) spliceBucket2(old *entry.NodeEntry, new *entry.NodeEntry) (oldBucket *Bucket, newBucket *Bucket, err error) {
	if old != nil {
		head, index := old, uint64(0)
		item, err := chm.initEntries(head, index)
		if err != nil {
			return nil, nil, err
		}
		oldBucket = item
	}

	if new != nil {
		head, index := new, uint64(0)
		item, err := chm.initEntries(head, index)
		if err != nil {
			return nil, nil, err
		}
		newBucket = item
	}
	return
}

func (chm *ConcurrentRawHashMap) indexBucket(idx int, tab *[]uintptr) (bucket *Bucket, outIndex bool) {
	if idx >= cap(*tab) {
		return nil, false
	}
	_, _, bucket = chm.tabAt(tab, uint64(idx))
	if bucket != nil && bucket.forwarding && chm.transferIndex >= 0 {
		return chm.indexBucket(idx, bucket.newBuckets)
	}
	return bucket, true
}

func (chm *ConcurrentRawHashMap) ForEach(fun func(key, val []byte) error) error {
	bucket, exist := chm.indexBucket(0, chm.buckets)
	for i := 1; exist; i++ {
		if bucket != nil {
			if tree := bucket.Tree; bucket.isTree && tree != nil {
				if err := chm.TreeForEach(fun, tree.GetRoot()); err != nil {
					return err
				}
			} else {
				for cNode := bucket.Head; cNode != nil; cNode = cNode.Next {
					if err := fun(cNode.Key, cNode.Value); err != nil {
						return err
					}
				}
			}
		}
		bucket, exist = chm.indexBucket(i, chm.buckets)
	}
	return nil
}

func (chm *ConcurrentRawHashMap) TreeForEach(fun func(key, val []byte) error, node *entry.NodeEntry) error {
	if node == nil {
		return nil
	}
	if err := fun(node.Key, node.Value); err != nil {
		return err
	}
	chm.TreeForEach(fun, node.Left())
	chm.TreeForEach(fun, node.Right())
	return nil
}

// BKDRHashWithSpread .   31 131 1313 13131 131313 etc..
func BKDRHashWithSpread(str []byte) uint64 {
	seed := uint64(131)
	hash := uint64(0)
	for i := 0; i < len(str); i++ {
		hash = (hash * seed) + uint64(str[i])
	}
	return hash ^ (hash>>16)&0x7FFFFFFF
}
