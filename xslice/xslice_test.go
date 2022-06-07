package xslice

import (
	"log"
	"testing"

	"github.com/heiyeluren/xds"

)

func TestNewXslice(t *testing.T) {
	xs := NewXslice(xds.String,10)
	log.Println(xs.s)
}

func TestXslice_Append(t *testing.T) {
	xs2 := NewXslice(xds.Int,1)
	log.Println(xs2.s)

	for i:=0;i<=30;i++ {
		xs2.Append(i)
	}

	log.Println(xs2.s)
}

func TestXslice_Set(t *testing.T) {
	xs3 := NewXslice(xds.Int,1)
	log.Println(xs3.s)

	err := xs3.Set(1,123)
	if err != nil{
		panic(err)
	}

	log.Println(xs3.s)

}

func TestXslice_Get(t *testing.T) {
	xs4 := NewXslice(xds.Int,1)
	err := xs4.Set(1,123)
	if err != nil{
		panic(err)
	}

	v,err := xs4.Get(1)

	if err !=nil{
		panic(err)
	}

	log.Println(v)
}

func TestXslice_Free(t *testing.T) {
	xs5 := NewXslice(xds.Int,1)
	err := xs5.Set(1,123)
	if err != nil{
		panic(err)
	}

	log.Println(xs5)

	xs5.Free()

	log.Println(xs5)
}

func TestXslice_ForEach(t *testing.T) {
	xs6 := NewXslice(xds.String,1)
	xs6.Append("aaaa")
	xs6.Append("bbb")
	xs6.Append("cccc")
	xs6.Append("cccc")
	xs6.Append("cccc")
	xs6.Append("cccc")
	xs6.Append("cccc")
	xs6.Append("cccc")
	xs6.Append("cccc")
	xs6.Append("eee")


	xs6.ForEach(func(i int, v []byte) error {
		log.Println("i: ",i, "v: ", string(v))
		return nil
	})
}

func TestXslice_Len(t *testing.T) {
	xs8 := NewXslice(xds.Int,1)
	log.Println(xs8.s)

	err := xs8.Set(1,123)
	if err != nil{
		panic(err)
	}

	log.Println(xs8.s)
	xs8.Set(2,123)
	xs8.Set(3,123)

	log.Println("len: ",xs8.Len())
}
