## XDS - eXtensible Data Structure <br />（第三方可扩展的Go语言中高性能数据结构和数据类型合集）

A third-party extensible collection of high-performance data structures and data types in Go

<br />

## XDS 介绍：（什么是Xds）
Xds - eXtensible Data Structure（第三方可扩展的Go语言中高性能数据结构和数据类型合集）

Xds主要是为了解决现有Go语言官方内置的各类数据结构性能在高并发场景中不尽如人意的情况而开发，核心主要是依赖于XMM内存管理库基础之上开发，保证了高性能和内存可控。

- XDS 集合目前主要包含：
- XMap - 高性能的类似 map/sync.map 的Map型数据结构类型 （已开源）
- XSlice - 高性能类似 slice 的数组型数据结构类型（开发中）
- XChannel - 高性能的channel管道类型（调研中）
- 更多...


<br />

## Xds - XMap 概要介绍：

XMap 是属于高性能开源Go数据结构Xds中的map数据结构类型的实现，主要是基于高性能内存管理库XMM基础之上进行的开发，主要弥补了Go内置map的无法并发读写，并且总体读写性能比较差的问题而开发。

<br />

### 为什么要设计XMap？

现有Golang中的map数据结构无法解决并发读写问题，Sync.map 并发性能偏差，针对这个情况，为XCache服务需要一个高性能、大容量、高并发、无GC的Map，所以开发实现 XMap。
针对我们需求调研了市场上主要的 hashmap 结构，不能满足我们性能和功能要求。

<br />

### XMap设计目标是什么？

要求设计一个可以并发读写不会出现panic，要求并发读写 200w+ OPS/s 的并发map结构。
（写20%，读80%场景；说明：go自带map读写性能在80w ops/s，大量并发读写下可能panic；sync.map 写入性能在 100w OPS/s）

<br />

### XMap的技术特点：

- 绝对高性能的map数据结构（map的3倍，sync.map 的2倍并发性能）
- 内部实现机制对比Go原生 map/sync.map 技术设计上要更细致，更考虑性能，使用包括开地址法，红黑树等等结构提升性能；
- 为了节约内存，初始的值比较低，但是依赖于XMM高性能内存扩容方式，能够快速进行内存扩容，保证并发写入性能
- 底层采用XMM内存管理，不会受到Go系统本身gc机制的卡顿影响，保证高性能；
- 提供API更具备扩展性，在一些高性能场景提供更多调用定制设置，并且能够同时支持map类型操作和底层hash表类型操作，适用场景更多；
- 其他特性

<br />

### XMap性能数据和实现对比：

Xmap目前并发读写场景下性能可以达到 200万 op/s，对比原生map单机性能80万 op/s，提升了3倍+，对比Go官方扩展库 sync.Map 性能有2倍的提升。

<br />

##### XMap与Go官方数据结构特点对比：(20%写入，80%读场景)

| map模块 | 性能数据<br /> | 加锁机制 | 底层数据结构 | 内存机制 |
|------|------|------|------|------|
|map | 80w+ read/s <br /> 并发读写会panic | 无 | Hashtable + Array | Go gc |
|sync.Map | 100w+ op/s | RWMutex | map | Go gc |
| Xds.XMap | 200w+ op/s | CAS + RWMutex | Hashtable + Array + RBTree | XMM |

<br />
<br />


## 如何使用XMap？

快速使用：

1. 下载对应包
```shell
go get -u github.com/heiyeluren/xds
go get -u github.com/heiyeluren/xmm
```


2. 快速包含调用库：

```go
import (
   xmm "github.com/heiyeluren/xmm"
   xds "github.com/heiyeluren/xds"
   xmap "github.com/heiyeluren/xds/xmap"
)

//创建一个XMM内存块
f := &xmm.Factory{}
mm, err := f.CreateMemory(0.75)

//构建一个 map[string]string 的xmap
m, err := xds.NewMap(mm, xmap.String, xmap.String)

//写入、读取、删除一个元素
err = m.Set("name", "heiyeluren")
ret, err := m.Get("name")
err = m.Remove("name")
//...
```


3. 执行对应代码
```shell
go run map-test.go
```

#### XMap各类API使用案例：

##### - [XMap使用示例](https://github.com/heiyeluren/xds/blob/main/example/xmap_test0.go)
- ...


以上代码案例执行输出：
<br />
<img src="https://raw.githubusercontent.com/heiyeluren/xds/main/docs/img/xds02.png" width="30%">

<br />


## XMap内部是如何实现的？

- XMap内部实现机制描述
- 敬请期待

<br />

<hr />

<br />
<br />
<br />

## XDS 项目开发者

| 项目角色      | 项目成员 |
| ----------- | ----------- |
| 项目发起人/负责人      | 黑夜路人( @heiyeluren ) <br />老张 ( @Zhang-Jun-tao )       |
| 项目开发者   | 老张 ( @Zhang-Jun-tao ) <br />黑夜路人( @heiyeluren ) <br /> Viktor ( @guojun1992 )        |

<br /> <br />

## XDS 技术交流

XDS 还在早期，当然也少不了一些问题和 bug，欢迎大家一起共创，或者直接提交 PR 等等。

欢迎加入XDS技术交流微信群，要加群，可以先添加如下微信让对方拉入群：<br />
（如无法看到图片，请手工添加微信： heiyeluren2017 ）

<img src=https://raw.githubusercontent.com/heiyeluren/xmm/main/docs/img/heiyeluren2017-wx.jpg width=40% />



<br />
<br />






