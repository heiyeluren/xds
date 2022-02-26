
# Xds - XMap技术设计与实现

## 一、XMap 背景目标

- 名词解释：
```
XMap - eXtensible Map Struct（高性能的第三方Map数据结构）
Map - Golang原生map数据结构
sync.Map - Golang原生并发map结构
```

#### XMap设计背景：
现有Golang中的map数据结构无法解决并发读写问题，Sync.map 并发性能偏差，针对这个情况，为XCache服务需要一个高性能、大容量、高并发、无GC的Map，所以开发实现 XMap。
针对我们需求调研了市场上主要的 hashmap 结构，不能满足我们性能和功能要求。

<br />

#### XMap设计目标：
要求设计一个可以并发读写不会出现panic，要求并发读写 200w+ OPS/s 的并发map结构。
（写20%，读80%场景；说明：go自带map读写性能在80w ops/s，大量并发读写下可能panic；sync.map 写入性能在 100w OPS/s）

<br />
<br />

## 二、XMap数据结构设计
<br />

为了保证读写性能的要求，同时保证内存的友好。我们采用了开链法方式来做为存储结构，如图为基本数据结构：
<br />

```go
type ConcurrentHashMap struct {
  size      uint64    //当前key的数量
  threshold uint64    // 扩容阈值
  initCap   uint64   //初始化bulks大小
  
  //off-heap
  bulks *[]uintptr    // 保存链表头结点指针的数组
  mm    xmm.XMemory   // 非堆的对象初始化器
  lock  sync.RWMutex  

  treeSize uint64    // 由链表转为红黑树的阈值

  //resize
  sizeCtl   int64     // 状态： -1 正在扩容

  nextBulks     *[]uintptr     //正在扩容的目标Bulks 
  transferIndex uint64         //扩容的索引
}
```

<br />

核心数据结构示例图：

<img src="https://raw.githubusercontent.com/heiyeluren/xds/main/docs/img/xmap01.png" width="80%">

- 核心说明：使用key的hash值计算出数组索引，每个数组中保存一个链表或者红黑树头结点，找到所处的索引，利用尾插发追加节点。每个节点中有key、value等数据。

<br />
<br />

## 三、XMap内部实现流程图

#### 3.1、Set 存储
<img src="https://raw.githubusercontent.com/heiyeluren/xds/main/docs/img/xmap02.png" width="80%">

<br />

#### 3.2、Get 查询
<img src="https://raw.githubusercontent.com/heiyeluren/xds/main/docs/img/xmap03.png" width="50%">
<br />

#### 3.3、Resize 扩容
<img src="https://raw.githubusercontent.com/heiyeluren/xds/main/docs/img/xmap04.png" width="90%">
<br />



