package xmap

import (
	"errors"
	"reflect"
	"unsafe"

	"github.com/heiyeluren/xmm"
)

// 定义 map 结构的类型
// map[keyKind]valKind

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

var InvalidType = errors.New("type Error") // 类型错误

type ConcurrentHashMap struct {
	keyKind Kind
	valKind Kind
	data    *ConcurrentRawHashMap
}

// NewDefaultConcurrentHashMap 类似于make(map[keyKind]valKind)
// mm 内存分配模块
// keyKind: map中key的类型
// valKind: map中value的类型
func NewDefaultConcurrentHashMap(mm xmm.XMemory, keyKind, valKind Kind) (*ConcurrentHashMap, error) {
	return NewConcurrentHashMap(mm, 16, 0.75, 8, keyKind, valKind)
}

// NewConcurrentHashMap 类似于make(map[keyKind]valKind)
// mm 内存分配模块
// keyKind: map中key的类型
// cap:初始化bucket长度
// fact:负载因子，当存放的元素超过该百分比，就会触发扩容。
// treeSize：bucket中的链表长度达到该值后，会转换为红黑树。
// valKind: map中value的类型
func NewConcurrentHashMap(mm xmm.XMemory, cap uintptr, fact float64, treeSize uint64, keyKind, valKind Kind) (*ConcurrentHashMap, error) {
	chm, err := NewConcurrentRawHashMap(mm, cap, fact, treeSize)
	if err != nil {
		return nil, err
	}
	return &ConcurrentHashMap{keyKind: keyKind, valKind: valKind, data: chm}, nil
}

func (chm *ConcurrentHashMap) Get(key interface{}) (val interface{}, keyExists bool, err error) {
	k, err := chm.Marshal(chm.keyKind, key)
	if err != nil {
		return nil, false, err
	}
	valBytes, exists, err := chm.data.Get(k)
	if err != nil {
		return nil, exists, err
	}
	ret, err := chm.UnMarshal(chm.valKind, valBytes)
	if err != nil {
		return nil, false, err
	}
	return ret, true, nil
}

func (chm *ConcurrentHashMap) Put(key interface{}, val interface{}) (err error) {
	k, err := chm.Marshal(chm.keyKind, key)
	if err != nil {
		return err
	}
	v, err := chm.Marshal(chm.valKind, val)
	return chm.data.Put(k, v)
}

func (chm *ConcurrentHashMap) Del(key interface{}) (err error) {
	k, err := chm.Marshal(chm.keyKind, key)
	if err != nil {
		return err
	}
	return chm.data.Del(k)
}

// 序列化，将来考虑基本类型一次访问
func (chm *ConcurrentHashMap) Marshal(kind Kind, content interface{}) (data []byte, err error) {
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

func (chm *ConcurrentHashMap) UnMarshal(kind Kind, data []byte) (content interface{}, err error) {
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
