package main

import (
	"fmt"
	"github.com/spf13/cast"
	"github.com/heiyeluren/xmm"
	"github.com/heiyeluren/xds"
	"github.com/heiyeluren/xds/xmap"
)

// TestMap testing
// -----------------------------------
// 把Xmap当做普通map来使用
// 说明：类型不限制，初始化必须设定好数据类型，写入数据必须与这个数据类型一致，类似于 map[KeyType]ValType 必须相互符合
// -----------------------------------
/*
目前支持的类似于 map[keyType][valType] key value 类型如下：
就是调用：m, err := xds.NewMap(mm, xmap.String, xmap.Int) 后面的两个 keyType 和 valType 类型定义如下：
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
*/
func TestMap(mm xmm.XMemory) {

	// 初始化xmap的时候必须制定key和val的数据类型，数据类型是在xmap中定义的
	// 构建一个 map[string]int 的xmap
	m, err := xmap.NewMap(mm, xds.String, xds.Int)
	if err != nil {
		panic("call NewMap() fail")
	}

	var (
		k11 string = "id"
		v11 int    = 9527
	)

	// set的时候需要关注类型是否跟初始化对象的时候一致
	err = m.Set(k11, v11)
	if err != nil {
		panic("XMap.Set fail")
	}
	fmt.Println("XMap.Set key: [", k11, "] success")

	// get数据不用关心类型，也不用做类型转换
	ret, exists, err := m.Get(k11)
	if err != nil {
		panic("XMap.Get fail")
	}
	fmt.Println("XMap.Get key: [", k11, "] , value: [", ret, "]")

	// Remove数据
	err = m.Remove(k11)
	if err != nil {
		panic("XMap.Remove fail")
	}
	fmt.Println("XMap.Remove key: [", k11, "] succes")
	ret, exists, err = m.Get(k11)
	if !exists {
		fmt.Println("XMap.Get key: [", k11, "] not found")
	}

	// 调用扩展的Map函数使用方法(可以获得更高性能)

	// 生成KV数据
	var (
		k22 = "name"
		v22 = "heiyeluren"
	)
	// 生成一个 map[string]string 数据结构，默认大小256个元素，占用了75%后进行map扩容(这个初始化函数可以获得更好性能，看个人使用场景）
	m, err = xmap.NewMapEx(mm, xds.String, xds.String, uintptr(256), 0.75)
	// set数据
	m.Set(k22, v22)
	// get数据
	ret, exists, err = m.Get(k22)
	fmt.Println("XMap.Get key: [", k22, "] , value: [", ret, "]")

}

// TestHashMap testing
// -----------------------------------
// 把Xmap当做普通hashmap来使用
// 说明：Key/Value 都必须是 []byte
// -----------------------------------
func TestHashMap(mm xmm.XMemory) {

	fmt.Println("===== XMap X(eXtensible) Raw Map (HashMap) example ======")

	hm, err := xmap.NewHashMap(mm)
	if err != nil {
		panic("call NewHashMap() fail")
	}

	var (
		k1 string = "name"
		v1 string = "heiyeluren"
		k2 string = "id"
		v2 uint32 = 9527
	)

	// 新增Key
	fmt.Println("----- XMap Set Key ------")
	err = hm.Set([]byte(k1), []byte(v1))
	if err != nil {
		panic("xmap.Set fail")
	}
	fmt.Println("Xmap.Set key: [", k1, "] success")
	err = hm.Set([]byte(k2), []byte(cast.ToString(v2)))
	if err != nil {
		panic("xmap.Set fail")
	}
	fmt.Println("Xmap.Set key: [", k2, "] success")

	// 读取Key
	fmt.Println("----- XMap Get Key ------")
	s1, exists, err := hm.Get([]byte(k1))
	if err != nil {
		panic("xmap.Get fail")
	}
	fmt.Println("Xmap.Get key: [", k1, "], value: [", cast.ToString(s1), "]")
	s2, exists, err := hm.Get([]byte(k2))
	if err != nil {
		panic("xmap.Get fail")
	}
	fmt.Println("Xmap.Get key: [", k2, "], value: [", cast.ToString(s2), "]")

	// 删除Key
	fmt.Println("\n----- XMap Remove Key ------")
	err = hm.Remove([]byte(k1))
	if err != nil {
		panic("xmap.Remove fail")
	}
	fmt.Println("Xmap.Remove key: [", k1, "]")
	s1, exists, err = hm.Get([]byte(k1))
	// fmt.Println(s1, exists, err)
	if !exists {
		fmt.Println("Xmap.Get key: [", k1, "] Not Found")
	}
	s2, exists, err = hm.Get([]byte(k2))
	if err != nil {
		panic("xmap.Get fail")
	}
	fmt.Println("Xmap.Get key: [", k2, "], value: [", cast.ToString(s2), "]")
	err = hm.Remove([]byte(k2))
	if err != nil {
		panic("xmap.Remove fail")
	}
	fmt.Println("Xmap.Remove key: [", k2, "]")
	s2, exists, err = hm.Get([]byte(k2))
	// fmt.Println(s1, exists, err)
	if !exists {
		fmt.Println("Xmap.Get key: [", k2, "] Not Found")
	}
	s1, exists, err = hm.Get([]byte(k1))
	// fmt.Println(s1, exists, err)
	if !exists {
		fmt.Println("Xmap.Get key: [", k1, "] Not Found")
	}

}

// xmap测试代码
func main() {
	f := &xmm.Factory{}
	mm, err := f.CreateMemory(0.75)
	if err != nil {
		panic("xmm.CreateConcurrentHashMapMemory fail")
	}
	fmt.Println("===== XMap X(eXtensible) Map example ======")

	// var NotFound = errors.New("not found")

	// 把Xmap当做普通map来使用
	TestMap(mm)

	// 把Xmap当做普通hashmap来使用
	TestHashMap(mm)

	fmt.Println("Xmap test case done.")

}
