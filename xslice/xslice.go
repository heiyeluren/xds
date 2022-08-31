package xslice

import (
	"errors"
	"math"
	"sync"

	"github.com/heiyeluren/xds"
)

var InvalidType = errors.New("type Error") // 类型错误

type _gap []byte
const _blockSize int = 8

type Xslice struct{
	s   [][_blockSize]_gap //[]*uintptr   改为[]数组。数组内存放基本类型结构体和指针。string的话，存放指针。

	//s  []uintptr   uintptr地址指向一个连续内存(数组)的头。
	// s 的创建和扩容，通过xmm的alloc(元素个数*8)。迁移完后销毁。
	// s 中的元素创建：通过xmm的alloc(元素个数*元素单位长度)。append到s中。
	// 基本类型存放：int、uint、直接放入到s的元素中。
	// 非基本类型存放：string、[]byte,先拷贝内存到xmm中，然后，将指针append到s的元素中。

	lock  *sync.RWMutex

	sizeCtl   int64 // -1 正在扩容

	//游标位置
	curRPos int     //读光标
	curWGapPos int  //写gap光标
	curWSlotPos int //写slot光标

	_sizeof int     //每个元素大小
	_stype xds.Kind //类型
	_len int        //元素个数计数

	_scap int       //容量
}

//func NewXslice(mm xmm.XMemory,_type Kind,_cap int) * Xslice {
func NewXslice(_type xds.Kind,_cap int) *Xslice {

	//计算分配slot数量，有余数会多分配一个
	slotCap := float64(_cap % _blockSize)
	var slotNum int64 = int64(math.Ceil(slotCap))

	if slotNum <= 0{
		slotNum = 1
	}

	slot := make([][8]_gap,slotNum,slotNum)
	return &Xslice{
		s: slot,
		lock: new(sync.RWMutex),
		_scap: _cap,
		_stype: _type,
	}
}

func(xs *Xslice) Append(v interface{})  (*Xslice,error) {
	xs.lock.Lock()
	defer xs.lock.Unlock()


	//判断类型：基本copy进去。
	// s中将要存放的uintptr拿到，uintptr为一个起始地址，加上一个index偏移量为写入地址。


	var gapPos int64 = 0

	slotCap := float64(xs.curWGapPos % _blockSize)

	gapPos = int64(slotCap)
	// 没有空位，扩容
	if slotCap == 0 && xs.curWGapPos >= _blockSize{
		newSlot := [_blockSize]_gap{}
		xs.s = append(xs.s,newSlot)
		xs.curWSlotPos += 1
	}
	b,err := xds.Marshal(xs._stype,v)
	if err != nil{
		return xs,err
	}

	xs.s[xs.curWSlotPos][gapPos] = b

	xs.curWGapPos += 1
	xs._len += 1

	return xs,nil
}

func (xs *Xslice) Set(n int,v interface{}) error {
	xs.lock.Lock()
	defer xs.lock.Unlock()

	var gapPos int64 = 0
	slotCap := float64(n % _blockSize)
	gapPos = int64(slotCap)

	// 没有空位，扩容
	if slotCap == 0 && xs.curWGapPos >= _blockSize{
		newSlot := [_blockSize]_gap{}
		xs.s = append(xs.s,newSlot)
		xs.curWSlotPos += 1
	}

	b,err := xds.Marshal(xs._stype,v)
	if err != nil{
		return err
	}

	xs.s[xs.curWSlotPos][gapPos] = b
	xs.curWGapPos += 1
	xs._len += 1

	return nil
}

func (xs *Xslice) Get(n int) (interface{}, error) {
	xs.lock.Lock()
	defer xs.lock.Unlock()

	var gapPos int64 = 0
	slotCap := float64(n % _blockSize)
	gapPos = int64(slotCap)
	b := xs.s[xs.curWSlotPos][gapPos]
	v,err := xds.UnMarshal(xs._stype,b)
	if err != nil{
		return nil,err
	}

	return v,nil
}

func (xs *Xslice) Free ()  {
	xs.lock.Lock()
	xs.curWGapPos = 0
	xs.curRPos = 0
	xs._scap = 0
	xs._sizeof = 0
	xs._len = 0
	xs.curWSlotPos = 0
	xs.s = nil
	xs.lock.Unlock()
}

func(xs *Xslice) ForEach(f func(i int, v interface{}) error) error{
	xs.lock.Lock()
	defer xs.lock.Unlock()

	var c int = 0
	for i,gaps := range xs.s {
		for _, b := range gaps {
			//判断坐标是否到头
			if xs.curWSlotPos == i && xs.curWGapPos == c{
				return nil
			}
			c = c + 1

			v,err := xds.UnMarshal(xs._stype,b)
			if err != nil{
				return nil
			}

			if err := f(c,v);err != nil{
				return err
			}
		}
	}

	return nil
}

//返回xslice长度
func (xs *Xslice) Len() int {
	xs.lock.Lock()
	defer xs.lock.Unlock()
	return xs._len
}

//@todo 把扩容逻辑封装起来
func (xs *Xslice) increase() *Xslice {
	return nil
}