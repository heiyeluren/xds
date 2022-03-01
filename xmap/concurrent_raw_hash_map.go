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
	"log"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
	"github.com/heiyeluren/xmm"
	"github.com/heiyeluren/xds/xmap/entry"
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
var _BucketPtrSize = unsafe.Sizeof(&Bucket{})
var _NodeEntrySize = unsafe.Sizeof(entry.NodeEntry{})
var _TreeSize = unsafe.Sizeof(entry.Tree{})
var uintPtrSize = uintptr(8)


// ConcurrentRawHashMap is a concurrent hash map with a fixed number of buckets.
// 清理方式：1、del清除entry(简单)
// 		2、bulks清除旧的（resize结束，并无get读引用【引入重试来解决该问题】）
// 		3、Bucket清除
// 		4、ForwardingBucket清除
// 		5、Tree 清除（spliceEntry2时候）
type ConcurrentRawHashMap struct {
	size      uint64
	threshold uint64
	initCap   uint64
	// off-heap
	bulks *[]uintptr
	mm    xmm.XMemory
	lock  sync.RWMutex

	treeSize uint64

	// resize
	sizeCtl   int64 // -1 正在扩容
	reSizeGen uint64

	nextBulks     *[]uintptr
	transferIndex uint64
}


// Bucket is a hash bucket.
type Bucket struct {
	forwarding bool // 已经迁移完成
	rwLock     sync.RWMutex
	index      uint64
	newBulks   *[]uintptr
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
	newBulks   *[]uintptr
}


// Snapshot 利用快照比对产生
type Snapshot struct {
	bulks     *[]uintptr
	sizeCtl   int64 // -1 正在扩容
	nextBulks *[]uintptr
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
	bulksPtr, err := mm.AllocSlice(uintPtrSize, cap, cap)
	if err != nil {
		return nil, err
	}
	bulks := (*[]uintptr)(bulksPtr)
	return &ConcurrentRawHashMap{bulks: bulks, initCap: uint64(cap), threshold: uint64(float64(cap) * fact),
		mm: mm, treeSize: treeSize}, nil
}

func (chm *ConcurrentRawHashMap) getBulk(h uint64, tab *[]uintptr) *Bucket {
	mask := uint64(cap(*tab) - 1)
	idx := h & mask
	_, _, bulk := chm.tabAt(tab, idx)
	if bulk != nil && bulk.forwarding && chm.transferIndex >= 0 {
		return chm.getBulk(h, bulk.newBulks)
	}
	return bulk
}

// Get Fetch key from hashmap
func (chm *ConcurrentRawHashMap) Get(key []byte) (val []byte, keyExists bool, err error) {
	h := BKDRHashWithSpread(key)
	bulk := chm.getBulk(h, chm.bulks)
	if bulk == nil {
		return nil, KeyNotExists, NotFound
	}
	if bulk.isTree {
		exist, value := bulk.Tree.Get(key)
		if !exist {
			return nil, KeyNotExists, NotFound
		}
		return value, KeyExists, nil
	}
	keySize := len(key)
	for cNode := bulk.Head; cNode != nil; cNode = cNode.Next {
		if keySize == len(cNode.Key) && bytes.Compare(key, cNode.Key) == 0 {
			return cNode.Value, KeyExists, nil
		}
	}
	return nil, KeyNotExists, NotFound
}
func (chm *ConcurrentRawHashMap) initForwardingEntries(newBulks *[]uintptr, index uint64) (*ForwardingBucket, error) {
	entriesPtr, err := chm.mm.Alloc(_ForwardingBucketSize)
	if err != nil {
		return nil, err
	}
	entries := (*ForwardingBucket)(entriesPtr)
	chm.assignmentForwardingEntries(newBulks, entries, index)
	return entries, nil
}

func (chm *ConcurrentRawHashMap) assignmentForwardingEntries(newBulks *[]uintptr, entries *ForwardingBucket, index uint64) {
	entries.newBulks = newBulks
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

func (chm *ConcurrentRawHashMap) helpTransform(entry *entry.NodeEntry, bulk *Bucket, tab *[]uintptr) (currentBulk *Bucket, init bool, currentTab *[]uintptr, err error) {
	if bulk != nil && bulk.forwarding {
		if err := chm.reHashSize(tab); err != nil {
			return nil, init, nil, err
		}
		tabPtr := bulk.newBulks
		bulk, swapped, err := chm.getAndInitBulk(entry, tabPtr)
		if err != nil {
			return nil, init, nil, err
		}
		if swapped {
			return nil, true, nil, err
		}
		return bulk, init, tabPtr, nil
	}
	return bulk, init, tab, nil
}

func (chm *ConcurrentRawHashMap) indexAndInitBulk(entry *entry.NodeEntry) (entries *Bucket, init bool, tabPtr *[]uintptr, err error) {
	tabPtr = chm.bulks
	bulk, swapped, err := chm.getAndInitBulk(entry, tabPtr)
	if err != nil {
		return nil, init, nil, err
	}
	if swapped {
		return bulk, true, tabPtr, nil
	}
	if bulk != nil && bulk.forwarding && chm.transferIndex >= 0 {
		bulk, swapped, tabPtr, err = chm.helpTransform(entry, bulk, tabPtr)
		if err != nil {
			return nil, init, nil, err
		}
		if swapped {
			return bulk, true, tabPtr, nil
		}
	}
	return bulk, init, tabPtr, nil
}

func (chm *ConcurrentRawHashMap) index(h uint64, length int) uint64 {
	idx := h & uint64(length-1)
	return idx
}

func (chm *ConcurrentRawHashMap) tabAt(bulks *[]uintptr, idx uint64) (*uintptr, uintptr, *Bucket) {
	addr := &((*bulks)[idx])
	ptr := atomic.LoadUintptr(addr)
	if ptr == 0 {
		return addr, ptr, nil
	}
	bulk := (*Bucket)(unsafe.Pointer(ptr))
	return addr, ptr, bulk
}

// cas 设置bulk
func (chm *ConcurrentRawHashMap) getAndInitBulk(entry *entry.NodeEntry, tabPtr *[]uintptr) (bulk *Bucket, swapped bool, err error) {
	h := entry.Hash
	idx := chm.index(h, cap(*tabPtr))
	// retry := 10改小后，出现该问题。
	addr, _, bulk := chm.tabAt(tabPtr, idx)
	if bulk != nil {
		return bulk, false, nil
	}
	entity, err := chm.initEntries(entry, idx)
	if err != nil {
		return nil, false, err
	}
	ptr := uintptr(unsafe.Pointer(entity))
	atomic.CompareAndSwapUintptr(addr, 0, ptr)
	bulk = (*Bucket)(unsafe.Pointer(atomic.LoadUintptr(addr)))
	return bulk, swapped, nil
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
		bulk, init, newTabPtr, err := chm.indexAndInitBulk(node)
		if err != nil {
			return nil, err
		}
		tabPtr = newTabPtr
		if init {
			break
		}
		if loop, err = chm.PutBulkValue(bulk, node, tabPtr); err != nil {
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
	tabPtr = chm.bulks
	for loop {
		idx := chm.index(h, cap(*tabPtr))
		_, _, bulk := chm.tabAt(tabPtr, idx)
		if bulk == nil {
			return nil
		}
		if bulk != nil && bulk.forwarding {
			if err := chm.reHashSize(tabPtr); err != nil {
				return err
			}
			tabPtr = bulk.newBulks
			idx = chm.index(h, cap(*tabPtr))
			_, _, bulk = chm.tabAt(tabPtr, idx)
			continue
		}
		var removeNode *entry.NodeEntry
		func() {
			// 删除bulk中的数目
			bulk.rwLock.Lock()
			defer bulk.rwLock.Unlock()
			_, _, newBulk := chm.tabAt(tabPtr, chm.index(h, cap(*tabPtr)))
			if newBulk != bulk || (bulk != nil && bulk.forwarding) {
				return
			}
			if bulk.isTree {
				if node := bulk.Tree.Delete(key); node != nil {
					removeNode = node
				}
			} else {
				keySize := len(key)
				var pre *entry.NodeEntry
				for cNode := bulk.Head; cNode != nil; cNode = cNode.Next {
					if keySize == len(cNode.Key) && bytes.Compare(key, cNode.Key) == 0 {
						removeNode = cNode
						if pre == nil {
							bulk.Head = cNode.Next
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

func (chm *ConcurrentRawHashMap) growTree(bulk *Bucket) error {
	if bulk.isTree || bulk.size < chm.treeSize {
		return nil
	}
	treePtr, err := chm.mm.Alloc(_TreeSize)
	if err != nil {
		return err
	}
	bulk.Tree = (*entry.Tree)(treePtr)
	bulk.Tree.SetComparator(entry.BytesAscSort)
	for node := bulk.Head; node != nil; {
		next := node.Next
		if err := bulk.Tree.Put(node); err != nil {
			return err
		}
		node = next
	}
	bulk.Head = nil
	bulk.isTree = true
	return nil
}

// PutBulkValue table的可见性问题，并发问题。
func (chm *ConcurrentRawHashMap) PutBulkValue(bulk *Bucket, node *entry.NodeEntry, tab *[]uintptr) (loop bool, err error) {
	bulk.rwLock.Lock()
	defer bulk.rwLock.Unlock()
	idx := chm.index(node.Hash, cap(*tab))
	_, _, newBulk := chm.tabAt(tab, idx)
	if newBulk != bulk || newBulk.forwarding {
		return true, nil
	}
	bulk = newBulk
	// 树化
	if err := chm.growTree(bulk); err != nil {
		return false, err
	}
	if bulk.isTree {
		if err := bulk.Tree.Put(node); err != nil {
			return false, err
		}
		return false, nil
	}
	key, val := node.Key, node.Value
	var last *entry.NodeEntry
	for node := bulk.Head; node != nil; node = node.Next {
		if len(key) == len(node.Key) && bytes.Compare(node.Key, key) == 0 {
			node.Value = val
			return false, nil
		}
		if node.Next == nil {
			last = node
		}
	}
	bulk.size += 1
	// 加入
	if last == nil {
		bulk.Head = node
		return false, nil
	}
	last.Next = node
	return false, nil
}

func (chm *ConcurrentRawHashMap) expandCap(tab *[]uintptr) (s *Snapshot, need bool) {
	old, size, transferIndex, threshold := uint64(cap(*chm.bulks)), atomic.LoadUint64(&chm.size),
		atomic.LoadUint64(&chm.transferIndex), atomic.LoadUint64(&chm.threshold)
	if size < threshold || transferIndex >= old {
		return nil, false
	}
	nextBulks, bulks, sizeCtl := chm.nextBulks, chm.bulks, atomic.LoadInt64(&chm.sizeCtl)
	// 正在扩容
	if sizeCtl < 0 && nextBulks != nil && cap(*nextBulks) == int(old)*2 {
		if sizeCtl >= 0 {
			return nil, false
		}
		// 当前扩容状态正确，开始扩容
		if unsafe.Pointer(tab) == unsafe.Pointer(bulks) && nextBulks != nil {
			if atomic.CompareAndSwapInt64(&chm.sizeCtl, sizeCtl, sizeCtl+1) {
				return &Snapshot{bulks: bulks, sizeCtl: sizeCtl, nextBulks: nextBulks}, true
			}
		}
		return nil, false
	}
	// 未开始扩容的判断
	if sizeCtl >= 0 && (old<<1) > uint64(sizeCtl) && unsafe.Pointer(tab) == unsafe.Pointer(bulks) {
		newSizeCtl := chm.resizeStamp(old) + 2
		swapped := atomic.CompareAndSwapInt64(&chm.sizeCtl, sizeCtl, newSizeCtl)
		// 开始扩容
		if swapped {
			newCap := old << 1
			bulksPtr, err := chm.mm.AllocSlice(uintPtrSize, uintptr(newCap), uintptr(newCap))
			if err != nil {
				log.Printf("ConcurrentRawHashMap chm.mm.Alloc newCap:%d  err:%s \n", newCap, err)
				return nil, false
			}
			nextBulks = (*[]uintptr)(bulksPtr)
			chm.nextBulks = nextBulks
			chm.transferIndex = 0
			return &Snapshot{bulks: bulks, sizeCtl: sizeCtl, nextBulks: nextBulks}, true
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
	currentCap := uint64(cap(*snapshot.bulks))
	bulks := snapshot.nextBulks
	// 取当前内容oldBulks，在oldBulks中利用bulk的lock来避免同时操作。
	err := chm.reSizeBulks(snapshot)
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
		oldBulks, tabPtr := chm.bulks, unsafe.Pointer(&chm.bulks)
		bulksAddr := (*unsafe.Pointer)(tabPtr)
		old := (*reflect.SliceHeader)(atomic.LoadPointer(bulksAddr))
		if old.Cap != int(currentCap) {
			return nil
		}
		if atomic.CompareAndSwapPointer(bulksAddr, unsafe.Pointer(old), unsafe.Pointer(bulks)) {
			// 更换内容赋值
			chm.nextBulks = nil
			newCap := currentCap << 1
			atomic.StoreInt64(&chm.sizeCtl, int64(newCap))
			chm.threshold = chm.threshold << 1
			chm.reSizeGen += 1
			xmm.TestBbulks = uintptr(unsafe.Pointer(chm.bulks))
			// fmt.Println(unsafe.Pointer(chm.bulks), xmm.TestBbulks, len(*chm.bulks))
			if err := chm.freeBuckets(oldBulks); err != nil {
				log.Printf("freeBuckets err:%s\n", err)
			}
			return err
		}
		return nil
	}()
}

/*
todo 清除内存
bulks清除旧的（resize结束，并无get读引用【引入重试来解决该问题】）
3、Bucket清除
4、ForwardingBucket清除*/
func (chm *ConcurrentRawHashMap) freeBuckets(bulks *[]uintptr) error {
	for _, ptr := range *bulks {
		if ptr < 1 {
			continue
		}
		if err := chm.mm.Free(ptr); err != nil {
			log.Printf("freeBuckets Free element(%d) err:%s\n", ptr, err)
		}
	}
	if err := chm.mm.Free(uintptr(unsafe.Pointer(bulks))); err != nil {
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

func (chm *ConcurrentRawHashMap) reSizeBulks(s *Snapshot) error {
	newBulks, oldBulks, currentCap := s.nextBulks, s.bulks, uint64(cap(*s.bulks))
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
				addr := &((*oldBulks)[index])
				entries := (*Bucket)(unsafe.Pointer(atomic.LoadUintptr(addr)))
				if entries == nil {
					forwardingEntries, err := chm.initForwardingEntries(newBulks, index)
					if err != nil {
						log.Printf("ERROR reSizeBulk initForwardingEntries err:%s\n", err)
						return err
					}
					ptr := uintptr(unsafe.Pointer(forwardingEntries))
					if atomic.CompareAndSwapUintptr(addr, 0, ptr) {
						break
					} else {
						entries = (*Bucket)(unsafe.Pointer(atomic.LoadUintptr(addr)))
					}
				}
				if loop, err = chm.reSizeBulk(entries, s); err != nil {
					log.Printf("ERROR reSizeBulk  oldEntries err:%s\n", err)
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

func (chm *ConcurrentRawHashMap) reSizeBulk(entries *Bucket, s *Snapshot) (loop bool, err error) {
	entries.rwLock.Lock()
	defer entries.rwLock.Unlock()
	oldBulks, newBulks := s.bulks, s.nextBulks
	currentCap := uint64(cap(*s.bulks))
	tabPtr, nextTabPtr := unsafe.Pointer(s.bulks), unsafe.Pointer(s.nextBulks)
	_, _, currentEntries := chm.tabAt(oldBulks, entries.index)
	// 判断newBulks为chm.newBulks,判断newBulks为chm.newBulks
	if entries.forwarding || tabPtr != unsafe.Pointer(chm.bulks) ||
		nextTabPtr != unsafe.Pointer(chm.nextBulks) || entries != currentEntries {
		return false, nil
	}
	if entries != nil && ((!entries.isTree && entries.Head == nil) || (entries.isTree && entries.Tree == nil)) {
		entries.newBulks = newBulks
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
		(*newBulks)[oldIndex] = uintptr(unsafe.Pointer(oldBucket))
	}
	if newBucket != nil {
		newBucket.index = newIndex
		(*newBulks)[newIndex] = uintptr(unsafe.Pointer(newBucket))
	}
	entries.newBulks = newBulks
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
		// todo 删除现在的tree内容
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

// BKDRHashWithSpread .
func BKDRHashWithSpread(str []byte) uint64 {
	seed := uint64(131) // 31 131 1313 13131 131313 etc..
	hash := uint64(0)
	for i := 0; i < len(str); i++ {
		hash = (hash * seed) + uint64(str[i])
	}
	return hash ^ (hash>>16)&0x7FFFFFFF
}

