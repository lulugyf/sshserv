package hh

import (
	"encoding/json"
	"fmt"
	"github.com/lulugyf/sshserv/hdfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"syscall"
	"github.com/lulugyf/sshserv/hdfs/intrnl/protocol/hadoop_hdfs"
)


func TestMain(m *testing.M) {
	fmt.Printf("---- in TestMain\n")
	m.Run()
}
var h *HConnection

func TestInitialization(t *testing.T) {
	fmt.Printf("---- in TestInitialization\n")
	//h = &HConnection{}
	//err := h.MkHdfsClient("/home/laog/dev/hdfs_e3base/etc", "e3base", "")
	//assert.Nil(t, err)
}

func Test123(t *testing.T) {
	x := strings.SplitN("1 2 3", " ", 2)
	fmt.Printf("%d\n", len(x))
	if u, err := user.LookupId("1000"); err == nil {
		fmt.Printf("username of id=1000 => [%v]\n", u.Username)
	}else{
		fmt.Printf("can not found user\n")
	}

}

func Test2(t *testing.T) {
	hosts := os.Getenv("hosts")
	if hosts != "" {
		var dhosts map[string]string
		err := json.Unmarshal([]byte(hosts), &dhosts)
		if err == nil {
			fmt.Printf(" host[MyHost]=[%s]\n", dhosts["MyHost"])
			fmt.Printf("xx=[%s]\n", dhosts["xx"])
		}else{
			fmt.Printf(" json parse failed: %v\n", err)
		}
	}else{
		fmt.Println("hosts not found")
	}
}

func _TestReadFile1(t *testing.T) {
	fmt.Println(h.FileRead("/user/iasp/dm_entry.py"))
	//username, err := hdfs.UserName()
	//if err != nil {
	//	t.Fatal(err)
	//}
	//fmt.Printf("username: %v\n", username)
}
func _TestDirs(t *testing.T) {
	tpath := "/user/iasp/123"
	err := h.client.Mkdir(tpath, 0755)
	assert.Nil(t, err)

	err = h.client.Remove(tpath)
	assert.Nil(t, err)
	fmt.Printf("mkdir ok!!!\n")
}

func _TestListDir(t *testing.T) {
	files, err := h.client.ReadDir("/user/iasp")
	assert.Nil(t, err)
	var s1 *syscall.Stat_t
	var s2 *hadoop_hdfs.HdfsFileStatusProto
	var ff *hdfs.FileInfo
	for i, fi := range files {
		if i == 3 {
			ff = fi.(*hdfs.FileInfo)
			ff1 := sftpFileInfo{h:ff}
			//fmt.Printf("--fi: %s %v %v %v  <mode>: %s  sys[%v]\n", fi.Name(), fi.IsDir(), fi.Size(), fi.ModTime(),
			//	fi.Mode().String(), fi.Sys())
			s2 = ff.Sys().(*hadoop_hdfs.HdfsFileStatusProto)
			fmt.Printf("--%v  %v \n    %v, \n  %v\n", reflect.TypeOf(s2), reflect.TypeOf(fi), s2, ff1.Sys())
			fmt.Printf("    %v\n", ff1.Mode())
		}
	}
	fmt.Println()

	files, err = ioutil.ReadDir("/user/iasp")
	for _, fi := range files {
		s1 = fi.Sys().(*syscall.Stat_t)
		fmt.Printf("--%v  %v \n    %v\n", reflect.TypeOf(s1), reflect.TypeOf(fi), s1)
		fmt.Printf("    %v\n", s1.Nlink)
	}


}

var cachedClients = make(map[string]*hdfs.Client)

func getClient(t *testing.T) *hdfs.Client {
	return getClientForUser(t, "")
}

func getClientForUser(t *testing.T, user string) *hdfs.Client {
	if c, ok := cachedClients[user]; ok {
		return c
	}

	nn := os.Getenv("HADOOP_NAMENODE")
	if nn == "" {
		t.Fatal("HADOOP_NAMENODE not set")
	}

	client, err := hdfs.New(nn)
	if err != nil {
		t.Fatal(err)
	}

	cachedClients[user] = client
	return client
}

func touch(t *testing.T, path string) {
	c := getClient(t)

	err := c.CreateEmptyFile(path)
	if err != nil && !os.IsExist(err) {
		t.Fatal(err)
	}
}

func mkdirp(t *testing.T, path string) {
	c := getClient(t)

	err := c.MkdirAll(path, 0644)
	if err != nil && !os.IsExist(err) {
		t.Fatal(err)
	}
}

func baleet(t *testing.T, path string) {
	c := getClient(t)

	err := c.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func assertPathError(t *testing.T, err error, op, path string, wrappedErr error) {
	require.NotNil(t, err)

	expected := &os.PathError{op, path, wrappedErr}
	require.Equal(t, expected.Error(), err.Error())
	require.Equal(t, expected, err)
}

func _TestNewWithMultipleNodes(t *testing.T) {
	nn := os.Getenv("HADOOP_NAMENODE")
	if nn == "" {
		t.Fatal("HADOOP_NAMENODE not set")
	}
	_, err := hdfs.NewClient(hdfs.ClientOptions{
		Addresses: []string{"localhost:80", nn},
	})
	assert.Nil(t, err)
}

func _TestNewWithFailingNode(t *testing.T) {
	_, err := hdfs.New("localhost:80")
	assert.NotNil(t, err)
}

func _TestReadFile(t *testing.T) {
	client := getClient(t)

	bytes, err := client.ReadFile("/_test/foo.txt")
	assert.NoError(t, err)
	assert.EqualValues(t, "bar\n", string(bytes))
}


func _TestCopyToLocal(t *testing.T) {
	client := getClient(t)

	dir, _ := ioutil.TempDir("", "hdfs-test")
	tmpfile := filepath.Join(dir, "foo.txt")
	err := client.CopyToLocal("/_test/foo.txt", tmpfile)
	require.NoError(t, err)

	f, err := os.Open(tmpfile)
	require.NoError(t, err)

	bytes, _ := ioutil.ReadAll(f)
	assert.EqualValues(t, "bar\n", string(bytes))
}

func _TestCopyToRemote(t *testing.T) {
	client := getClient(t)

	baleet(t, "/_test/copytoremote.txt")
	err := client.CopyToRemote("test/foo.txt", "/_test/copytoremote.txt")
	require.NoError(t, err)

	bytes, err := client.ReadFile("/_test/copytoremote.txt")
	require.NoError(t, err)

	assert.EqualValues(t, "bar\n", string(bytes))
}
