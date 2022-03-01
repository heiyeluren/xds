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
	"errors"
	"reflect"
	"unsafe"
	// "github.com/heiyeluren/xmm"
)

// 定义XDS共用的常量和一些基础函数
//调用方法：
/*
	//包含
	import(
		"github.com/heiyeluren/xds"
	)

	//结构体中使用
	type ConcurrentHashMap struct {
		keyKind xds.Kind
		valKind xds.Kind
		data    *ConcurrentRawHashMap
	}

	//一般数据结构中使用
	m, err := xds.NewMap(mm, xds.String, xds.Int)

	//序列化处理 (其他类型到 []byte)
	data, err := xds.Marshal(xds.String, "heiyeluren")

	// 反序列化处理 (从 []byte到原始类型)
	str, err := xds.UnMarshal(xds.String, data)

*/



//========================================================
//
// XDS 常量和数据结构定义区
// XDS constant and data structure definition
//
//========================================================

// XDS 中数据类型的定义
// map[keyKind]valKind == xds.NewMap(mm, xds.keyKind, xds.valKind)
// make([]typeKind, len) == xds.NewSlice(mm, xds.dataKind)

type Kind uint

const (
	Invalid Kind = iota
	Bool
	Int
	Int8
	Int16
	Int32
	Int64
	Uint
	Uint8
	Uint16
	Uint32
	Uint64
	Uintptr
	Float32
	Float64
	Complex64
	Complex128
	Array
	Chan
	Func
	Interface
	Map
	Ptr
	ByteSlice
	String
	Struct
	UnsafePointer
)

// 类型错误
// type error
var InvalidType = errors.New("Kind type Error")

// type ConcurrentHashMap struct {
// 	keyKind Kind
// 	valKind Kind
// 	data    *ConcurrentRawHashMap
// }



//========================================================
//
// XDS 常用函数定义区
// XDS common function definition area
//
//========================================================

// 针对一些Kind数据类型的序列化
// Serialization for some kind data types
func Marshal(kind Kind, content interface{}) (data []byte, err error) {
	switch kind {
	case String:
		data, ok := content.(string)
		if !ok {
			return nil, InvalidType
		}
		sh := (*reflect.StringHeader)(unsafe.Pointer(&data))
		return *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data: sh.Data, Len: sh.Len, Cap: sh.Len})), nil
	case ByteSlice:
		data, ok := content.([]byte)
		if !ok {
			return nil, InvalidType
		}
		return data, nil
	case Int:
		h, ok := content.(int)
		if !ok {
			return nil, InvalidType
		}
		return (*[8]byte)(unsafe.Pointer(&h))[:], nil
	case Uintptr:
		h, ok := content.(uintptr)
		if !ok {
			return nil, InvalidType
		}
		return (*[8]byte)(unsafe.Pointer(&h))[:], nil
	}
	return
}

// 针对一些Kind数据类型的反序列化
// Deserialization for some kind data types
func UnMarshal(kind Kind, data []byte) (content interface{}, err error) {
	switch kind {
	case String:
		return *(*string)(unsafe.Pointer(&data)), nil
	case Uintptr:
		sh := (*reflect.SliceHeader)(unsafe.Pointer(&data))
		return *(*uintptr)(unsafe.Pointer(sh.Data)), nil
	case Int:
		sh := (*reflect.SliceHeader)(unsafe.Pointer(&data))
		return *(*int)(unsafe.Pointer(sh.Data)), nil
	case ByteSlice:
		return data, nil
	}
	return
}


