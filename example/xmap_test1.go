package main

import (
	"fmt"
	"github.com/heiyeluren/xmm"
	"github.com/heiyeluren/xds"
	"github.com/heiyeluren/xds/xmap"
)

//------------------
// xmap快速使用案例
//------------------

func main() {

	//创建XMM内存块
	f := &xmm.Factory{}
	mm, err := f.CreateMemory(0.75)
	if err != nil {
		panic("error")
	}

	//用xmap创建一个map，结构类似于 map[string]uint，操作类似于 m: = make(map[string]uint)
	m, err := xmap.NewMap(mm, xds.String, xds.Int)
	if err != nil {
		panic("error")
	}
	// 调用Set()给xmap写入数据
	err = m.Set("k1", 1)
	err = m.Set("k2", 2)
	err = m.Set("k3", 3)
	err = m.Set("k4", 4)
	err = m.Set("k5", 5)

	// 调用Get()读取单个数据
	ret, exists, err := m.Get("k1")
	if exists == false {
		panic("key not found")
	}
	fmt.Printf("Get data key:[%s] value:[%s] \n", "k1", ret)

	// 使用Each()访问所有xmap元素
	// 在遍历操作中，让全局变量可以在匿名函数中访问（如果需要使用外部变量，可以像这样）
	gi := 1
	//调用 Each 遍历函数
	m.Each(func(key, val interface{}) error {
		//针对每个KV进行操作，比如打印出来
		fmt.Printf("For each XMap all key:[%s] value:[%s] \n", key, val)

		//外部变量使用操作
		gi++
		return nil
	})
	fmt.Printf("For each success, var[gi], val[%s]\n", gi);

	//使用Len()获取xmap元素总数量
	len := m.Len()
	fmt.Printf("Xmap item size:[%s]\n", len);

}
