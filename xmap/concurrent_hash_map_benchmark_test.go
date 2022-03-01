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

package xmap

import (
	"fmt"
	"math/rand"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"
	"unsafe"
	// "xds/xmap/entry"
	// "github.com/spf13/cast"
	"github.com/heiyeluren/xmm"
	"github.com/heiyeluren/xds/xmap/entry"
)

// 1000000000
func BenchmarkCHM_Put(b *testing.B) {
	f := &xmm.Factory{}
	mm, err := f.CreateMemory(0.75)
	if err != nil {
		b.Fatal(err)
	}
	chm, err := NewConcurrentRawHashMap(mm, 16, 2, 8)
	if err != nil {
		b.Fatal(err)
	}
	keys := make([]string, 10000000)
	for i := 0; i < 10000000; i++ {
		keys[i] = strconv.Itoa(rand.Int())
	}
	length := len(keys)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte(keys[rand.Int()%length])
		if err := chm.Put(key, key); err != nil {
			b.Error(err)
		}
	}
}

func BenchmarkCHM_Get(b *testing.B) {
	f := &xmm.Factory{}
	mm, err := f.CreateMemory(0.75)
	if err != nil {
		b.Fatal(err)
	}
	chm, err := NewConcurrentRawHashMap(mm, 16, 2, 8)
	if err != nil {
		b.Fatal(err)
	}
	keys := make([]string, 8000000)
	for i := 0; i < 8000000; i++ {
		keys[i] = strconv.Itoa(rand.Int())
	}
	length := len(keys)
	for _, key := range keys {

		if err := chm.Put([]byte(key), []byte(key)); err != nil {
			b.Error(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := keys[rand.Int()%length]
		if _, exist, err := chm.Get([]byte(key)); err != nil || !exist {
			b.Error(err)
		}
	}
}

func Test_CHM_Concurrent_Get(t *testing.T) {
	// Init()
	f := &xmm.Factory{}
	mm, err := f.CreateMemory(0.75)
	if err != nil {
		t.Fatal(err)
	}
	chm, err := NewConcurrentRawHashMap(mm, 16, 2, 8)
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 8000000)
	for i := 0; i < 8000000; i++ {
		keys[i] = strconv.Itoa(rand.Int() % 800000)
	}
	for _, key := range keys {
		if err := chm.Put([]byte(key), []byte(key)); err != nil {
			t.Error(err)
		}
	}
	var wait sync.WaitGroup
	wait.Add(10)
	fmt.Println("开始")
	t1 := time.Now()
	for j := 0; j < 10; j++ {
		go func(z int) {
			defer wait.Done()
			start, end := z*800000, (z+1)*800000
			for _, s := range keys[start:end] {
				if err := chm.Put([]byte(s), []byte(s)); err != nil {
					t.Error(err)
				}
			}
		}(j)
	}
	wait.Wait()
	fmt.Println(len(keys), time.Now().Sub(t1), len(keys))
	<-time.After(time.Minute)
}

func TestGoCreateEntry(t *testing.T) {
	var wait sync.WaitGroup
	wait.Add(10)
	node := &entry.NodeEntry{}
	// nodes := make([]*entry.NodeEntry, 8000000)
	tt := time.Now()
	for j := 0; j < 10; j++ {
		go func(z int) {
			defer wait.Done()
			for i := 0; i < 800000; i++ {
				node.Key = []byte("keyPtr")
				node.Value = []byte("valPtr")
				node.Hash = 12121
				// nodes[z*800000+i] = node
			}
		}(j)
	}
	wait.Wait()
	fmt.Println(time.Now().Sub(tt))
}

func TestCreateEntry(t *testing.T) {
	f := &xmm.Factory{}
	mm, err := f.CreateMemory(0.75)
	if err != nil {
		t.Fatal(err)
	}
	entryPtr, err := mm.Alloc(_NodeEntrySize)
	if err != nil {
		t.Fatal(err)
	}
	/* 80445440 \ 283598848 \ 228630528 \ 267530240 \97157120 \ 129908736
	   keyPtr, valPtr, err := mm.From2("hjjhj", "jjjshjfhsdf")
	   if err != nil {
	   	t.Fatal(err)
	   }*/
	pageNum := float64(uintptr(entryPtr)) / 4096.0
	fmt.Println(uintptr(entryPtr), pageNum, (pageNum+1)*4096 > float64(uintptr(entryPtr)+_NodeEntrySize))
	node := (*entry.NodeEntry)(entryPtr)
	tt := time.Now()
	var wait sync.WaitGroup
	wait.Add(10)
	for j := 0; j < 10; j++ {
		go func(z int) {
			defer wait.Done()
			for i := 0; i < 8000000; i++ {
				node.Key = []byte("keyPtr")
				node.Value = []byte("valPtr")
				node.Hash = 12121
			}
		}(j)
	}
	wait.Wait()
	fmt.Println(time.Now().Sub(tt))
}

// todo 优秀一些，使用这种方式
func TestFieldCopy(t *testing.T) {
	f := &xmm.Factory{}
	mm, err := f.CreateMemory(0.75)
	if err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	wait.Add(10)
	for j := 0; j < 10; j++ {
		go func(z int) {
			defer wait.Done()
			for i := 0; i < 800000; i++ {
				entryPtr, err := mm.Alloc(_NodeEntrySize)
				if err != nil {
					t.Fatal(err)
				}
				keyPtr, valPtr, err := mm.Copy2([]byte("hjjhj"), []byte("jjjshjfhsdf"))
				if err != nil {
					t.Fatal(err)
				}
				source := entry.NodeEntry{Value: keyPtr, Key: valPtr, Hash: 12312}
				offset := unsafe.Offsetof(source.Next) // 40
				srcData := (*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data: uintptr(unsafe.Pointer(&source)), Len: int(offset), Cap: int(offset)}))
				dstData := (*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data: uintptr(entryPtr), Len: int(offset), Cap: int(offset)}))
				copy(*dstData, *srcData)
			}
		}(j)
	}
	wait.Wait()
}
