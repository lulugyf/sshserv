package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	fmt.Printf("---- in TestMain\n")
	m.Run()
}

func TestInitialization(t *testing.T) {
	fmt.Printf("---- in TestInitialization\n")
	//h = &HConnection{}
	//err := h.MkHdfsClient("/home/laog/dev/hdfs_e3base/etc", "e3base", "")
	//assert.Nil(t, err)
}

func _Test123(t *testing.T) {
	//c, err := Connect("172.18.231.76", 22, "mci", "/home/laog/.ssh/id_rsa")
	//c, err := Connect("localhost", 2022, "_base_", "/home/laog/.ssh/id_rsa")
	c, err := Connect("localhost", 2022, "laog", "/home/laog/.ssh/id_rsa")
	if err != nil {
		t.Errorf("connect failed %v", err)
	}
	defer c.Close()
	c.Upload("/tmp/xx.txt", "/tmp/x1.txt")

}
func _Test22(t *testing.T) {
	s := strings.SplitN("1.1.1.1:22@/path/to/file",  "@", 2)
	fmt.Println(len(s), s[1], )

	var k interface{} = nil
	if x, OK := k.(string); OK {
		fmt.Println(x)
	}else{
		fmt.Println("no ok")
	}
}

//func TestFolderLocal2Remote(t *testing.T) {
//	err := FileTran("/tmp/x1", "localhost:2022@/tmp/x5", "/home/laog/.ssh/id_rsa")
//	if err != nil {
//		t.Errorf("local to remote failed [%v]\n", err)
//	}
//}
//
//func TestFolderRemote2Remote(t *testing.T) {
//	err := FileTran("localhost:2022@/tmp/x5", "localhost:2022@/tmp/x7", "/home/laog/.ssh/id_rsa")
//	if err != nil {
//		t.Errorf("remote to remote failed [%v]\n", err)
//	}
//}
//
//func TestFolderRemote2Local(t *testing.T) {
//	err := FileTran("localhost:2022@/tmp/x5", "/tmp/x8", "/home/laog/.ssh/id_rsa")
//	if err != nil {
//		t.Errorf("remote to local failed [%v]\n", err)
//	}
//}

//func TestFolderLocal2Local(t *testing.T) {
//	err := FileTran("/tmp/x5", "/tmp/x9", "/home/laog/.ssh/id_rsa")
//	if err != nil {
//		t.Errorf("local to local failed [%v]\n", err)
//	}
//}
//
//
//func TestFileLocal2Remote(t *testing.T) {
//	err := FileTran("/tmp/xx.txt", "localhost:2022@/tmp/xx1.txt", "/home/laog/.ssh/id_rsa")
//	if err != nil {
//		t.Errorf("[f]local to remote failed [%v]\n", err)
//	}
//}
//
//func TestFileRemote2Remote(t *testing.T) {
//	err := FileTran("localhost:2022@/tmp/xx1.txt", "localhost:2022@/tmp/xx2.txt", "/home/laog/.ssh/id_rsa")
//	if err != nil {
//		t.Errorf("[f]remote to remote failed [%v]\n", err)
//	}
//}

func TestFileRemote2Local(t *testing.T) {
	err := FileTran("localhost:2022@/tmp/xx1.txt", "/tmp/xx3.txt", "/home/laog/.ssh/id_rsa")
	if err != nil {
		t.Errorf("[f]remote to local failed [%v]\n", err)
	}
}

//func TestFileLocal2Local(t *testing.T) {
//	err := FileTran("/tmp/xx.txt", "/tmp/xx4.txt", "/home/laog/.ssh/id_rsa")
//	if err != nil {
//		t.Errorf("[f]local to local failed [%v]\n", err)
//	}
//}
