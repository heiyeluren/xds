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
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"
	// "xds/xmap/entry"
	"github.com/heiyeluren/xmm"
	"github.com/spf13/cast"
	"github.com/heiyeluren/xds/xmap/entry"
)

func TestMap(t *testing.T) {
	t1 := time.Now()
	var data sync.Map
	var wait sync.WaitGroup
	wait.Add(10)
	for h := 0; h < 10; h++ {
		go func() {
			defer wait.Done()
			for i := 0; i < 1000000; i++ {
				key := cast.ToString(i)
				data.Store(key, key)
			}
		}()
	}
	wait.Wait()
	fmt.Println(time.Now().Sub(t1))
}

func TestConcurrentRawHashMap_Performance(t *testing.T) {
	Init()
	f := &xmm.Factory{}
	mm, err := f.CreateMemory(0.6)
	if err != nil {
		t.Fatal(err)
	}
	chm, err := NewConcurrentRawHashMap(mm, 16, 0.75, 8)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("http://localhost:6060/debug/pprof/profile")
	// http://localhost:6060/debug/pprof/profile
	for i := 0; i < 1000000000; i++ {
		key := cast.ToString(i)
		if err := chm.Put([]byte(key), []byte(key)); err != nil {
			t.Error(err)
		}
	}
}

func TestConcurrentRawHashMap_Function_Second(t *testing.T) {
	f := &xmm.Factory{}
	mm, err := f.CreateMemory(0.75)
	if err != nil {
		t.Fatal(err)
	}
	chm, err := NewConcurrentRawHashMap(mm, 16, 4, 8)
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	wait.Add(10)
	keys := make(chan string, 1000000)
	quit := make(chan bool, 1)
	go func() {
		<-time.After(time.Second * 22)
		t2 := time.Now()
		for {
			key, ok := <-keys
			if !ok {
				fmt.Println("read time:", time.Now().Sub(t2))
				quit <- true
				return
			}
			val, exist, err := chm.Get([]byte(cast.ToString(key)))
			if bytes.Compare(val, []byte(cast.ToString(key))) != 0 || !exist {
				t.Error(err, key)
			}
		}
	}()
	t1 := time.Now()
	for j := 0; j < 10; j++ {
		go func(z int) {
			defer wait.Done()
			for i := 0; i < 100000; i++ {
				key := cast.ToString(i + (z * 10000))
				if err := chm.Put([]byte(key), []byte(key)); err != nil {
					t.Error(err)
				} else {
					keys <- key
				}
			}
		}(j)
	}
	wait.Wait()
	fmt.Println(len(keys), time.Now().Sub(t1), len(keys))
	close(keys)
	f.PrintStatus()

	<-quit
	<-time.After(time.Second * 10)
}

func TestConcurrentRawHashMap_Function1(t *testing.T) {
	Init()
	// runtime.GOMAXPROCS(16)
	f := &xmm.Factory{}
	mm, err := f.CreateMemory(0.75)
	if err != nil {
		t.Fatal(err)
	}
	chm, err := NewConcurrentRawHashMap(mm, 16, 4, 8)
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	wait.Add(10)
	keys := make(chan string, 8000000)
	quit := make(chan bool, 1)
	go func() {
		<-time.After(time.Second * 10)
		t2 := time.Now()
		for {
			key, ok := <-keys
			if !ok {
				fmt.Println("read time:", time.Now().Sub(t2))
				quit <- true
				return
			}
			val, exist, err := chm.Get([]byte(key))
			if bytes.Compare(val, []byte(key)) != 0 || !exist {
				t.Fatal(err, key)
			}
		}
	}()
	t1 := time.Now()
	for j := 0; j < 10; j++ {
		go func(z int) {
			defer wait.Done()
			for i := 0; i < 800000; i++ {
				key := cast.ToString(i + (z * 10000))
				if err := chm.Put([]byte(key), []byte(key)); err != nil {
					t.Error(err)
				} else {
					keys <- key
				}
			}
		}(j)
	}
	wait.Wait()
	fmt.Println(len(keys), time.Now().Sub(t1), len(keys))
	close(keys)
	<-quit
}

func TestMMM(t *testing.T) {
	// /usr/local/go/src/runtime
	filepath.Walk("/usr/local/go/src/runtime", func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		if path[len(path)-3:] != ".go" {
			return nil
		}
		bytes, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		for _, s1 := range strings.Split(string(bytes), "\n") {
			s1 = strings.TrimSpace(s1)
			if len(s1) < 3 || s1[:len("//")] == "//" || strings.Contains(s1, " = ") || strings.Contains(info.Name(), "_test.go") {
				continue
			}
			for _, s2 := range strings.Split(s1, " ") {
				s := strings.TrimSpace(s2)
				if len(s) < 1 {
					continue
				}
				if int(s[0]) <= 64 || int(s[0]) >= 91 {
					continue
				}
				fmt.Println( /*path, info.Name(),*/ s, s1)
			}
		}
		return nil
	})
}

type A struct {
	lock sync.RWMutex
	age  int
}

func TestSizeOf(t *testing.T) {
	fmt.Println(unsafe.Sizeof(sync.RWMutex{}), unsafe.Sizeof(A{}))
	fmt.Println(unsafe.Sizeof(Bucket2{}))
}

type Bucket2 struct {
	forwarding bool // 已经迁移完成
	rwlock     sync.RWMutex
	index      uint64
	newBulks   *[]uintptr
	Head       *entry.NodeEntry
	/*
		Tree       *entry.Tree
		isTree     bool
		size       uint64
	*/
}

func TestMMCopyString(t *testing.T) {
	f := &xmm.Factory{}
	mm, err := f.CreateMemory(0.75)
	if err != nil {
		t.Fatal(err)
	}
	var item1, item2 string
	for i := 0; i < 10000000; i++ {
		item1, item2, err = mm.From2("sdsddsds", "sdsddsdssdsds")
		if err != nil {
			t.Error(err)
		}
	}
	fmt.Println(item1, item2)
}

func TestGoCopyString(t *testing.T) {
	f := &xmm.Factory{}
	mm, err := f.CreateMemory(0.75)
	if err != nil {
		t.Fatal(err)
	}
	var item1, item2 string
	for i := 0; i < 10000000; i++ {
		item1, item2, err = mm.From2("sdsddsds", "sdsddsdssdsds")
		if err != nil {
			t.Error(err)
		}
	}
	fmt.Println(item1, item2)
}

type KV struct {
	K string
	V string
}

func TestDataSet(t *testing.T) {
	fmt.Println(string(make([]byte, 100)))
	num, maxLen := 10000000, 1000
	kvs := make([]*KV, num)
	for i := 0; i < num; i++ {
		r := rand.New(rand.NewSource(time.Now().UnixNano())).Int()
		k := RandString(r % maxLen)
		kvs[i] = &KV{
			K: cast.ToString(r % num),
			V: k,
		}
	}
	fmt.Println(RandString(1000))
}

func RandString(len int) string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	bytes := make([]byte, len)
	for i := 0; i < len; i++ {
		b := r.Intn(26) + 65
		bytes[i] = byte(b)
	}
	return string(bytes)
}

/*func TestDecode(t *testing.T) {
	node := entry.NodeEntry{}
	sss := node.Int64Encode(212121)
	fmt.Println(node.Int64Decode(sss))

}*/
