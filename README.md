## XDS - eXtensible Data Structure Sets <br />(第三方可扩展的 Golang 高性能数据结构和数据类型合集)

A third-party extensible collection of high-performance data structures and data types in Go

- [XDS - eXtensible Data Structure <br />（第三方可扩展的 Go 语言中高性能数据结构和数据类型合集）](#xds---extensible-data-structure-第三方可扩展的-go-语言中高性能数据结构和数据类型合集)
- [XDS 介绍：（什么是 Xds）](#xds-介绍什么是-xds)
- [XDS - XMap 概要介绍](#xds---xmap-概要介绍)
  - [为什么要设计 XMap？](#为什么要设计-xmap)
  - [XMap 设计目标是什么？](#xmap-设计目标是什么)
  - [XMap 的技术特点](#xmap-的技术特点)
  - [XMap 性能数据和实现对比](#xmap-性能数据和实现对比)
    - [XMap 与 Go 官方数据结构特点对比：(20% 写入，80% 读场景)](#xmap-与-go-官方数据结构特点对比20-写入80-读场景)
- [如何使用 XMap？](#如何使用-xmap)
  - [XMap 各类 API 使用案例](#xmap-各类-api-使用案例)
    - [- XMap 使用示例](#--xmap-使用示例)
- [XMap 内部是如何实现的？](#xmap-内部是如何实现的)
- [XDS 项目开发者](#xds-项目开发者)
- [XDS 技术交流](#xds-技术交流)

<br />

## XDS 介绍：（什么是 Xds）

XDS - eXtensible Data Structure（第三方可扩展的 Go 语言中高性能数据结构和数据类型合集）

XDS 主要是为了解决现有 Go 语言官方内置的各类数据结构性能在高并发场景中不尽如人意的情况而开发，核心主要是依赖于 [XMM](https://github.com/heiyeluren/xmm) 内存管理库基础之上开发，保证了高性能和内存可控。

XDS 集合目前主要包含：
- <b>XMap</b> - 高性能的类似 map/sync.map 的 Map 型数据结构类型（<b>已开源</b>）
- <b>XSlice</b>  - 高性能类似 slice 的数组型数据结构类型（<b>开发中</b>）
- <b>XChannel</b>  - 高性能的 channel 管道类型结构（<b>调研中</b>）
- 更多...

<br />

<hr />

<br />

## XDS - XMap 概要介绍

XMap 是属于高性能开源 Go 数据结构 Xds 中的 map 数据结构类型的实现，主要是基于高性能内存管理库 [XMM](https://github.com/heiyeluren/xmm) 基础之上进行的开发，主要弥补了 Go 内置 map 的无法并发读写，并且总体读写性能比较差的问题而开发。

<br />

### 为什么要设计 XMap？

现有 Golang 中的 map 数据结构无法解决并发读写问题，Sync.map 并发性能偏差，针对这个情况，各种高性能底层服务需要一个高性能、大容量、高并发、无 GC 的 Map，所以开发实现 XMap。
针对我们需求调研了市场上主要的 hashmap 结构，不能满足我们性能和功能要求。

<br />

### XMap 设计目标是什么？

要求设计一个可以并发读写不会出现 panic，要求并发读写 200w+ OPS/s 的并发 map 结构。
（写 20%，读 80% 场景；说明：go 自带 map 读写性能在 80w ops/s，大量并发读写下可能 panic；sync.map 写入性能在 100w OPS/s）

<br />

### XMap 的技术特点

- 绝对高性能的 map 数据结构（map 的 3 倍，sync.map 的 2 倍并发性能）
- 内部实现机制对比 Go 原生 map/sync.map 技术设计上要更细致，更考虑性能，使用包括开地址法，红黑树等等结构提升性能；
- 为了节约内存，初始的值比较低，但是依赖于 XMM 高性能内存扩容方式，能够快速进行内存扩容，保证并发写入性能
- 底层采用 XMM 内存管理，不会受到 Go 系统本身 GC 机制的卡顿影响，保证高性能；
- 提供 API 更具备扩展性，在一些高性能场景提供更多调用定制设置，并且能够同时支持 map 类型操作和底层 hash 表类型操作，适用场景更多；
- 其他特性

<br />

### XMap 性能数据和实现对比

XMap 目前并发读写场景下性能可以达到 200 万 op/s，对比原生 map 单机性能 80 万 op/s，提升了 3 倍 +，对比 Go 官方扩展库 sync.Map 性能有 2 倍的提升。

<br />

#### XMap 与 Go 官方数据结构特点对比：(20% 写入，80% 读场景)

| map 模块 | 性能数据<br /> | 加锁机制 | 底层数据结构 | 内存机制 |
|------|------|------|------|------|
|map | 80w+ read/s <br /> 并发读写会 panic | 无 | Hashtable + Array | Go gc |
|sync.Map | 100w+ op/s | RWMutex | map | Go gc |
| Xds.XMap | 200w+ op/s | CAS + RWMutex | Hashtable + Array + RBTree | XMM |

<br />
<br />

## 如何使用 XMap？

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

// 创建一个 XMM 内存块
f := &xmm.Factory{}
mm, err := f.CreateMemory(0.75)

// 构建一个 map[string]string 的 xmap
m, err := xds.NewMap(mm, xmap.String, xmap.String)

// 写入、读取、删除一个元素
err = m.Set("name", "heiyeluren")
ret, err := m.Get("name")
err = m.Remove("name")
//...
```

3. 执行对应代码

```shell
go run map-test.go
```

<br />

### XMap 各类 API 使用案例

#### - [XMap 使用示例](https://github.com/heiyeluren/xds/blob/main/example/xmap_test0.go) - 

 - 更多案例（期待）

以上代码案例执行输出：
<br />
<img src="https://raw.githubusercontent.com/heiyeluren/xds/main/docs/img/xds02.png" width="30%">

<br />

## XMap 内部是如何实现的？

#### - [《Xds-XMap技术设计与实现》](https://github.com/heiyeluren/xds/blob/main/docs/Xmap-Implement.md) -

<br />

- 参考：[《Go map 内部实现机制一》](https://www.jianshu.com/p/aa0d4808cbb8)  |  [《Go map 内部实现二》](https://zhuanlan.zhihu.com/p/406751292) | [《Golang sync.Map 性能及原理分析》](https://blog.csdn.net/u010853261/article/details/103848666)
- 其他

<br />

<hr />

<br />
<br />
<br />

## XDS 项目开发者

| 项目角色      | 项目成员 |
| ----------- | ----------- |
| 项目发起人/负责人      | 黑夜路人 ( @heiyeluren ) <br />老张 ( @Zhang-Jun-tao )       |
| 项目开发者   | 老张 ( @Zhang-Jun-tao ) <br />黑夜路人 ( @heiyeluren ) <br /> Viktor ( @guojun1992 )        |

<br /> <br />

## XDS 技术交流

XDS 还在早期，当然也少不了一些问题和 bug，欢迎大家一起共创，或者直接提交 PR 等等。

欢迎加入 XDS 技术交流微信群，要加群，可以先添加如下微信让对方拉入群：<br />
（如无法看到图片，请手工添加微信： heiyeluren2017 ）

<img src=https://raw.githubusercontent.com/heiyeluren/xmm/main/docs/img/heiyeluren2017-wx.jpg width=40% />



<br />
<br />
