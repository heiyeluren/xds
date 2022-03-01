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

package xds

import (
	"github.com/heiyeluren/xmm"

	"github.com/heiyeluren/xds/xmap"
)

// XMap is a map of maps.
// ------------------------------------------------
//  当做map[]来使用的场景
// 本套API主要是提供给把Xmap当做map来使用的场景
// ------------------------------------------------
type XMap struct {
	chm *xmap.ConcurrentHashMap
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
func NewMap(mm xmm.XMemory, keyKind xmap.Kind, valKind xmap.Kind) (*XMap, error) {
	chm, err := xmap.NewDefaultConcurrentHashMap(mm, keyKind, valKind)
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
func NewMapEx(mm xmm.XMemory, keyKind xmap.Kind, valKind xmap.Kind, capSize uintptr, factSize float64) (*XMap, error) {
	chm, err := xmap.NewConcurrentHashMap(mm, capSize, factSize, 8, keyKind, valKind)
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

// RawMap .
// ------------------------------------------------
//  当做原生Hash Map来使用场景
// 本套API主要是提供给把Xmap当做Hash表来使用的场景
// ------------------------------------------------
// 定义xmap的入口主结构
type RawMap struct {
	chm *xmap.ConcurrentRawHashMap
}

// NewHashMap returns a new RawMap.
//  初始化调用xmap - hashmap 生成对象 mm 是XMM的内存池对象
func NewHashMap(mm xmm.XMemory) (*RawMap, error) {
	chm, err := xmap.NewDefaultConcurrentRawHashMap(mm)
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
