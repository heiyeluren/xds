package main

import (
	"fmt"
//	"github.com/heiyeluren/xmm"
	"github.com/heiyeluren/xds"
	"github.com/heiyeluren/xds/xslice"
)

//---------------------
// xslice快速使用案例
//---------------------

func main() {

	//创建XSlice对象, 类似于 s0 []Int64 = make([]Int64, 1)
	s0 := xslice.NewXslice(xds.Int,1)

	//Set压入数据, 类似于 s0 :=[] int { 11, 22, 33 } 
	s0.Set(1, 11)
	s0.Set(2, 22)
	s0.Set(3, 33)

	//Get读取一个数据
	vi,nil := s0.Get(1)
	fmt.Println("XSlice get data 1: ", vi)
	vi,nil = s0.Get(2)
	fmt.Println("XSlice get data 2: ", vi)
	vi,nil = s0.Get(3)
	fmt.Println("XSlice get data 3: ", vi)

	//读取整个slice长度, 类似于 len()
	fmt.Println("XSlice data size: ", s0.Len(), "\n")

	//释放资源
	s0.Free()

	//批量压入Int ( 类似于 s1 []Int64 = make([]Int64, 1) )
	s1 := xslice.NewXslice(xds.Int, 10)
	for i:=50; i<=55; i++ {
		s1.Append(i)
	}
	//使用ForEach读取所有数据
	s1.ForEach(func(i1 int, v1 []byte) error {
		fmt.Println("XSlice foreach data i: ",i1, " v: ", string(v1))
		return nil
	})
// 	fmt.Println(s1.s)

	//读取整个slice长度, 类似于 len()
	fmt.Println("XSlice data size: ", s1.Len(), "\n")

	//使用 Append压入一批String数据 ( 类似于 s2 []String = make([]String, 1) )
	s2 := xslice.NewXslice(xds.String, 10)
	s2.Append("aaa")
	s2.Append("bbb")
	s2.Append("ccc")
	s2.Append("ddd")
	s2.Append("eee")

	//使用ForEach读取所有数据
	s2.ForEach(func(i2 int, v2 []byte) error {
		fmt.Println("XSlice foreach data i: ", i2, " v: ", string(v2))
		return nil
	})

	//读取整个slice长度, 类似于 len()
	fmt.Println("XSlice data size: ", s2.Len(), "\n")

	//释放资源
	s2.Free()



}
