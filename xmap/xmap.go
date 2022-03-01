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
	// "xds"
	"github.com/heiyeluren/xmm"
	"github.com/heiyeluren/xds"
)


// XMap is a map of maps.
// ------------------------------------------------
//  当做map[]来使用的场景
// 本套API主要是提供给把Xmap当做map来使用的场景
// ------------------------------------------------

// Xmap struct
type XMap struct {
	chm *ConcurrentHashMap
}

// NewMap returns a new XMap.
// 初始化调用xmap生成对象
//	mm 是XMM的内存池对象
//	keyKind 是要生成 map[keyType]valType 中的 keyType
//	valKind 是要生成 map[keyType]valType 中的 valType
//	说明：keyKind / valKind 都是直接调用xmap中对应的kind类型，必须提前初始化写入
//
// 本函数调用参考：
// 生成一个 map[int]string 数据结构，默认大小16个元素，占用了75%后进行map扩容
//	m, err := xds.NewMapEx(mm, xmap.Int, xmap.String)
func NewMap(mm xmm.XMemory, keyKind xds.Kind, valKind xds.Kind) (*XMap, error) {
	chm, err := NewDefaultConcurrentHashMap(mm, keyKind, valKind)
	if err != nil {
		return nil, err
	}
	return &XMap{chm: chm}, nil
}

// NewMapEx returns a new XMap.
//  初始化调用xmap生成对象 - 可扩展方法
// 	mm 是XMM的内存池对象
// 	keyKind 是要生成 map[keyType]valType 中的 keyType
// 	valKind 是要生成 map[keyType]valType 中的 valType
// 	说明：keyKind / valKind 都是直接调用xmap中对应的kind类型，必须提前初始化写入
// 	capSize 是默认初始化整个map的大小，本值最好是 2^N 的数字比较合适（2的N次方）；
// 			这个值主要提升性能，比如如果你预估到最终会有128个元素，可以初始化时候设置好，这样Xmap不会随意的动态扩容（默认值是16），提升性能
// 	factSize 负载因子，当存放的元素超过该百分比，就会触发扩容；建议值是0.75 (75%)，这就是当存储数据容量达到75%会触发扩容机制；
//
// 本函数调用参考：
// 	//生成一个 map[int]string 数据结构，默认大小256个元素，占用了75%后进行map扩容
// 	m, err := xds.NewMapEx(mm, xmap.Int, xmap.String, (uintptr)256, 0.75)
func NewMapEx(mm xmm.XMemory, keyKind xds.Kind, valKind xds.Kind, capSize uintptr, factSize float64) (*XMap, error) {
	chm, err := NewConcurrentHashMap(mm, capSize, factSize, 8, keyKind, valKind)
	if err != nil {
		return nil, err
	}
	return &XMap{chm: chm}, nil
}

// Set 写入数据
func (xm *XMap) Set(key interface{}, val interface{}) (err error) {
	return xm.chm.Put(key, val)
}

// Remove 删除数据
func (xm *XMap) Remove(key interface{}) (err error) {
	return xm.chm.Del(key)
}

// Get 数据
func (xm *XMap) Get(key interface{}) (val interface{}, keyExists bool, err error) {
	return xm.chm.Get(key)
}


// RawMap - Hashmap 
// ------------------------------------------------
//  当做原生Hash Map来使用场景
// 本套API主要是提供给把Xmap当做Hash表来使用的场景
// ------------------------------------------------
// 定义xmap的入口主结构
type RawMap struct {
	chm *ConcurrentRawHashMap
}

// NewHashMap returns a new RawMap.
//  初始化调用xmap - hashmap 生成对象 mm 是XMM的内存池对象
func NewHashMap(mm xmm.XMemory) (*RawMap, error) {
	chm, err := NewDefaultConcurrentRawHashMap(mm)
	if err != nil {
		return nil, err
	}
	return &RawMap{chm: chm}, nil
}

// Set 写入数据
func (xm *RawMap) Set(key []byte, val []byte) error {
	return xm.chm.Put(key, val)
}

// Remove 删除数据
func (xm *RawMap) Remove(key []byte) error {
	return xm.chm.Del(key)
}

// Get 数据
func (xm *RawMap) Get(key []byte) ([]byte, bool, error) {
	return xm.chm.Get(key)
}





// The data structure and interface between xMAP and the bottom layer
// ------------------------------------------------
//  xmap与底层交互的数据结构和接口
// 
//   主要提供给上层xmap api调用，完成一些转换工作
// ------------------------------------------------


//底层交互数据结构（带有传入数据类型保存）
type ConcurrentHashMap struct {
	keyKind xds.Kind
	valKind xds.Kind
	data    *ConcurrentRawHashMap
}


// NewDefaultConcurrentHashMap 类似于make(map[keyKind]valKind)
// mm 内存分配模块
// keyKind: map中key的类型
// valKind: map中value的类型
func NewDefaultConcurrentHashMap(mm xmm.XMemory, keyKind, valKind xds.Kind) (*ConcurrentHashMap, error) {
	return NewConcurrentHashMap(mm, 16, 0.75, 8, keyKind, valKind)
}


// NewConcurrentHashMap 类似于make(map[keyKind]valKind)
// mm 内存分配模块
// keyKind: map中key的类型
// cap:初始化bucket长度
// fact:负载因子，当存放的元素超过该百分比，就会触发扩容。
// treeSize：bucket中的链表长度达到该值后，会转换为红黑树。
// valKind: map中value的类型
func NewConcurrentHashMap(mm xmm.XMemory, cap uintptr, fact float64, treeSize uint64, keyKind, valKind xds.Kind) (*ConcurrentHashMap, error) {
	chm, err := NewConcurrentRawHashMap(mm, cap, fact, treeSize)
	if err != nil {
		return nil, err
	}
	return &ConcurrentHashMap{keyKind: keyKind, valKind: valKind, data: chm}, nil
}


//Get
func (chm *ConcurrentHashMap) Get(key interface{}) (val interface{}, keyExists bool, err error) {
	// k, err := chm.Marshal(chm.keyKind, key)
	k, err := xds.RawToByte(chm.keyKind, key)
	if err != nil {
		return nil, false, err
	}
	valBytes, exists, err := chm.data.Get(k)
	if err != nil {
		return nil, exists, err
	}
	// ret, err := chm.UnMarshal(chm.valKind, valBytes)
	ret, err := xds.ByteToRaw(chm.valKind, valBytes)
	if err != nil {
		return nil, false, err
	}
	return ret, true, nil
}


//Put
func (chm *ConcurrentHashMap) Put(key interface{}, val interface{}) (err error) {
	// k, err := chm.Marshal(chm.keyKind, key)
	k, err := xds.RawToByte(chm.keyKind, key)
	if err != nil {
		return err
	}
	// v, err := chm.Marshal(chm.valKind, val)
	v, err := xds.RawToByte(chm.valKind, val)
	return chm.data.Put(k, v)
}

//Del
func (chm *ConcurrentHashMap) Del(key interface{}) (err error) {
	// k, err := chm.Marshal(chm.keyKind, key)
	k, err := xds.RawToByte(chm.keyKind, key)
	if err != nil {
		return err
	}
	return chm.data.Del(k)
}


