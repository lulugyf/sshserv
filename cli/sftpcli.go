package main

import (
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"flag"
	//"bufio"
	"fmt"
	"path/filepath"

	"crypto/aes"
	//"encoding/hex"
)

type Cli struct {
	Ssh  *ssh.Client
	Sftp *sftp.Client
}

func (c *Cli) Connect(remote string, port int, user, pass string) {

	auths := []ssh.AuthMethod{ssh.Password(pass)}
	_, err := os.Stat(pass) // if os.IsNotExists(err)
	if err == nil {
		pemBytes, err := ioutil.ReadFile(pass)
		signer, err := ssh.ParsePrivateKey(pemBytes)
		if err == nil {
			auths = []ssh.AuthMethod{ssh.PublicKeys(signer)}
		}
	}
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		//HostKeyCallback: ssh.FixedHostKey(hostKey),
	}

	// connect
	addr := fmt.Sprintf("%s:%d", remote, port)
	log.Printf("addr: %s\n", addr)
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		log.Fatal("connect failed: ", err)
	} else {
		log.Printf("ssh connected.")
	}
	c.Ssh = conn

	// create new SFTP client
	client, err := sftp.NewClient(conn)
	if err != nil {
		//log.Printf("sftp.NewClient failed")
		log.Fatal("sftp failed: ", err)
	} else {
		log.Printf("sftp connected")
	}
	c.Sftp = client
}
func (c *Cli) Close() {
	c.Sftp.Close()
	c.Ssh.Close()
	fmt.Printf("ssh Closed\n")
}

func Connect(remote string, port int, user, pass string) (*Cli, error) {

	auths := []ssh.AuthMethod{ssh.Password(pass)}
	_, err := os.Stat(pass) // if os.IsNotExists(err)
	if err == nil {
		pemBytes, err := ioutil.ReadFile(pass)
		signer, err := ssh.ParsePrivateKey(pemBytes)
		if err == nil {
			auths = []ssh.AuthMethod{ssh.PublicKeys(signer)}
		}
	}
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		//HostKeyCallback: ssh.FixedHostKey(hostKey),
	}

	// connect
	addr := fmt.Sprintf("%s:%d", remote, port)
	//log.Printf("addr: %s\n", addr)
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		log.Fatal("connect failed:", err)
		return nil, err
	} else {
		log.Printf("ssh connected.")
	}

	// create new SFTP client
	client, err := sftp.NewClient(conn)
	if err != nil {
		//log.Printf("sftp.NewClient failed")
		log.Fatal("sftp failed:", err)
		return nil, err
	} else {
		log.Printf("sftp connected")
	}
	return &Cli{Ssh: conn, Sftp: client}, nil
}

func (c *Cli) Upload(local_file, remote_file string) {
	log.Printf("upload %s => %s", local_file, remote_file)
	// check if remote dir exists
	if strings.Index(remote_file, "/") >= 0 {
		pp := strings.Split(remote_file, "/")
		pdir := strings.Join(pp[:len(pp)-1], "/")
		_, err := c.Sftp.Stat(pdir)
		if err != nil {
			c.Sftp.MkdirAll(pdir)
		}
	}
	dstFile, err := c.Sftp.Create(remote_file)
	if err != nil {
		log.Fatal(err)
	}
	defer dstFile.Close()

	// create source file
	srcFile, err := os.Open(local_file)
	if err != nil {
		log.Fatal(err)
	}

	// copy source file to destination file
	bytes, err := io.Copy(dstFile, srcFile)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d bytes copied\n", bytes)
}
func (c *Cli) Download(remote_file, local_file string) {
	// check if local path exists
	if strings.Index(local_file, "/") >= 0 {
		pp := strings.Split(local_file, "/")
		pdir := strings.Join(pp[:len(pp)-1], "/")
		st, err := os.Stat(pdir)
		if err != nil {
			os.MkdirAll(pdir, os.FileMode(0700))
		} else {
			if !st.IsDir() {
				log.Println("local path is a file")
				return
			}
		}
	}
	// create destination file
	dstFile, err := os.Create(local_file)
	if err != nil {
		log.Fatal(err)
	}
	defer dstFile.Close()

	// open source file
	srcFile, err := c.Sftp.Open(remote_file)
	if err != nil {
		log.Fatal(err)
	}

	// copy source file to destination file
	bytes, err := io.Copy(dstFile, srcFile)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d bytes copied\n", bytes)

	// flush in-memory copy
	err = dstFile.Sync()
	if err != nil {
		log.Fatal(err)
	}
}

func (c *Cli) DownloadDir(remote_dir, local_dir string) {
	st, err := c.Sftp.Stat(remote_dir)
	if err != nil {
		log.Fatal(err)
	}
	pp := strings.Split(remote_dir, "/")
	if !st.IsDir() {
		c.Download(remote_dir, local_dir+"/"+pp[len(pp)-1])
		return
	}
	remote_plen := len(remote_dir) - len(pp[len(pp)-1]) - 1 // length of /tmp/
	walker := c.Sftp.Walk(remote_dir)
	for walker.Step() {
		local_file := local_dir + walker.Path()[remote_plen:]
		if walker.Stat().IsDir() {
			os.MkdirAll(local_file, os.FileMode(0700))
		} else {
			c.Download(walker.Path(), local_file)
		}
	}
}

/**
上传整个目录  /tmp/abc , /mci/xx => /mci/xx/abc
*/
func (c *Cli) UploadDir(local_dir, remote_dir string) {
	st, err := os.Stat(local_dir)
	if err != nil {
		log.Fatal(err)
	}
	pp := strings.Split(local_dir, "/")
	if !st.IsDir() {
		c.Upload(local_dir, remote_dir+"/"+pp[len(pp)-1])
		return
	}
	local_plen := len(local_dir) - len(pp[len(pp)-1]) - 1 // length of /tmp/
	log.Printf("local_plen: %d", local_plen)
	mywalkfunc := func(path string, info os.FileInfo, err error) error {
		remote_file := remote_dir + path[local_plen:] //
		//log.Printf("walk: %s -> %s  isdir: %v", path, remote_file, info.IsDir())
		if info.IsDir() {
			c.Sftp.MkdirAll(remote_file)
		} else {
			c.Upload(path, remote_file)
			//fmt.Printf("%s -> %s\n", path, remote_file)
		}
		return nil
	}
	filepath.Walk(local_dir, mywalkfunc)
}

func decodeAddr(addr string) string {
	// 编码方式: base64(aes("host:port:user:pass"))
	// key & iv 就先固定了

	pad := func(bb []byte) []byte {
		l := len(bb)
		b := 16 - l%16
		//fmt.Printf("   pad-- %d\n", b)
		size := l + b
		tmp := make([]byte, size)
		copy(tmp, bb)
		for i := l; i < size; i++ {
			tmp[i] = byte(b)
		}
		return tmp
	}
	unpad := func(bb []byte) string {
		b := int(bb[len(bb)-1])
		//fmt.Printf("   unpad-- %d\n", b)
		return string(bb[:len(bb)-int(b)])
	}
	AES_ENC := func(plain string, key []byte, iv []byte) (string, error) {
		origData := pad([]byte(plain))
		block, err := aes.NewCipher(key)
		if err != nil {
			return "", err
		}
		blockMode := cipher.NewCBCEncrypter(block, iv)
		crypted := make([]byte, len(origData))

		blockMode.CryptBlocks(crypted, origData)
		return base64.StdEncoding.EncodeToString(crypted), nil
	}
	AES_DEC := func(enc string, key []byte, iv []byte) (string, error) {
		block, err := aes.NewCipher(key)
		if err != nil {
			return "", err
		}
		blockMode := cipher.NewCBCDecrypter(block, iv)
		bb, err := base64.StdEncoding.DecodeString(enc)
		if err != nil {
			return "", err
		}
		decrypted := make([]byte, len(bb))
		blockMode.CryptBlocks(decrypted, bb)
		return unpad(decrypted), nil
	}

	key := []byte("thisis32bitlongpassphraseimusing")
	iv := []byte("1234567890abcdef")

	// import "math/rand"; iv := make([]byte, 16); rand.Read(iv)  // 这个来产生随机的iv

	//plain := "This is a secret123" // 16 bytes
	//str_enc, err := AES_ENC(plain, key, iv)
	//if err != nil {
	//	log.Fatal(err)
	//}
	//fmt.Println(str_enc)
	//fmt.Println(dec(str_enc, key, iv))
	if strings.Index(addr, ":") >= 0 {
		// 提供明文, 则加密处理
		str_enc, err := AES_ENC(addr, key, iv)
		if err != nil {
			log.Fatal(err)
		} else {
			return str_enc
		}
	} else {
		str_dec, err := AES_DEC(addr, key, iv)
		if err != nil {
			log.Fatal(err)
		} else {
			return str_dec
		}
	}

	return ""
}

/**
寻找本地目录中修改时间最新的文件或目录

return
  fileName
  error
*/
func findLocalNewestFile(local_dir string) (string, error) {
	st, err := os.Stat(local_dir)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", errors.New("local dir must be a folder")
	}
	files, err := ioutil.ReadDir(local_dir)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", errors.New("no file(s) found in local_dir")
	}
	f0 := files[0]
	for _, f := range files {
		if f.ModTime().After(f0.ModTime()) {
			f0 = f
		}
	}
	return fmt.Sprintf("%s/%s", local_dir, f0.Name()), nil
}

/**
寻找sftp目录中修改时间最新的文件或目录
*/
func findSftpNewestFile(sftp *sftp.Client, remote_dir string) (string, error) {
	st, err := sftp.Stat(remote_dir)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", errors.New("remote_path must be a folder")
	}
	files, err := sftp.ReadDir(remote_dir)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", errors.New("no file(s) found in remote_dir")
	}
	f0 := files[0]
	for _, f := range files {
		if f.ModTime().After(f0.ModTime()) {
			f0 = f
		}
	}
	return fmt.Sprintf("%s/%s", remote_dir, f0.Name()), nil
}

/**
上传文件
*/
func send(opts *OpFlag) {
	local_path, err := findLocalNewestFile(*opts.localDir)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("found path: %s\n", local_path)
	remote_path := fmt.Sprintf("/models/%s", *opts.idStr)
	c := Cli{}
	log.Println(decodeAddr(*opts.servAddr))
	xx := strings.SplitN(decodeAddr(*opts.servAddr), ":", 4)
	port, _ := strconv.Atoi(xx[1])
	c.Connect(xx[0], port, xx[2], xx[3])
	defer c.Close()

	c.UploadDir(local_path, remote_path)
}
func recv(opts *OpFlag) {
	c := Cli{}
	xx := strings.SplitN(decodeAddr(*opts.servAddr), ":", 4)
	port, _ := strconv.Atoi(xx[1])
	c.Connect(xx[0], port, xx[2], xx[3])
	defer c.Close()

	remote_dir := fmt.Sprintf("/models/%s", *opts.idStr)
	remote_path, err := findSftpNewestFile(c.Sftp, remote_dir)
	if err != nil {
		log.Fatal(err)
	}
	c.DownloadDir(remote_path, *opts.localDir)
}

type OpFlag struct {
	opType   *string // 操作类型  send or recv
	servAddr *string // sftp 服务器地址编码
	localDir *string // for send: 本地文件或目录位置, 在这个位置下, 选择修改时间最新的文件或目录进行上传
	// for recv: 拉取下来的文件保存的本地位置
	idStr *string // for send: 用于标识同一流程的id, 同一流程可以有多个任务, 其生成的模型文件传到同一个位置
	// for recv: 从服务器上拉取修改时间最新的文件和目录
	// sftp 上的地址为: /models/$idStr/
}

func parseArgs() *OpFlag {
	o := &OpFlag{
		opType:   flag.String("op", "", "opType of send | recv | enc"),
		servAddr: flag.String("serv", "", "encoded sftp server address, include login info"),
		localDir: flag.String("local", "", "local path of send or recv file/folder"),
		idStr:    flag.String("id", "", "Identify of workflow"),
	}
	flag.Parse()
	return o
}

func main_() {

	opts := parseArgs()
	if *opts.opType == "enc" {
		fmt.Println(decodeAddr(*opts.servAddr))
		return
	}
	if *opts.opType == "" || *opts.servAddr == "" || *opts.localDir == "" || *opts.idStr == "" {
		log.Fatal("Every option has no default value, must be specified.")
	}

	switch *opts.opType {
	case "send":
		send(opts)
	case "recv":
		recv(opts)

	}

	//c := Cli{}
	//c.Connect("172.18.231.76", 22, "mci", "Mci@321_5")
	////c.UploadDir("/tmp/abc", "/tmp")
	//c.DownloadDir("/tmp/abc", "/tmp/xx")
	//c.Close()
}

func connectRemote(connstr string, key_file string) (*Cli, string, error) {
	// 格式： 1.1.1.1:22@/path/to/file
	s := strings.SplitN(connstr, "@", 2)
	s1 := strings.SplitN(s[0], ":", 2)
	if len(s1) < 2 {
		return nil, "", errors.New("invalid addr format")
	}
	//fmt.Printf(" ----[%s]\n", connstr)
	port, err := strconv.Atoi(s1[1])
	if err != nil {
		return nil, "", err
	}
	co, err := Connect(s1[0], port, "_base_", key_file)
	if err != nil {
		return nil, "", err
	}

	return co, s[1], nil
}

func toRemote(cc interface{}, remote_file string, srcFile io.Reader) error {
	c, OK := cc.(*Cli)
	if !OK {
		return errors.New("invalid connection type!")
	}
	if strings.Index(remote_file, "/") >= 0 {
		pp := strings.Split(remote_file, "/")
		pdir := strings.Join(pp[:len(pp)-1], "/")
		_, err := c.Sftp.Stat(pdir)
		if err != nil {
			c.Sftp.MkdirAll(pdir)
		}
	}
	dstFile, err := c.Sftp.Create(remote_file)
	if err != nil {
		log.Fatal(err)
	}
	defer dstFile.Close()

	// copy source file to destination file
	_, err = io.Copy(dstFile, srcFile)
	return err
}

func toLocal(_ interface{}, remote_file string, srcFile io.Reader) error {
	if strings.Index(remote_file, "/") >= 0 {
		pp := strings.Split(remote_file, "/")
		pdir := strings.Join(pp[:len(pp)-1], "/")
		st, err := os.Stat(pdir)
		if err != nil {
			os.MkdirAll(pdir, os.FileMode(0700))
		} else {
			if !st.IsDir() {
				log.Println("local path is a file")
				return errors.New("dst file is a folder")
			}
		}
	}
	// create destination file
	dstFile, err := os.Create(remote_file)
	if err != nil {
		log.Fatal(err)
	}
	defer dstFile.Close()

	// copy source file to destination file
	_, err = io.Copy(dstFile, srcFile)
	return err
}

/**
  return
   1 - not found
   2 - dir
   3 - file
   -1 - failed
*/
func checkDstPath(dst interface{}, dstPath string) int {
	if d, OK := dst.(*Cli); OK {
		if st, err := d.Sftp.Stat(dstPath); err == nil {
			if st.IsDir() {
				return 2
			} else {
				return 3
			}
		} else if os.IsNotExist(err) {
			return 1
		}
	} else {
		if st, err := os.Stat(dstPath); err == nil {
			if st.IsDir() {
				return 2
			} else {
				return 3
			}
		} else if os.IsNotExist(err) {
			return 1
		}
	}
	return -1
}

type toFunc func(interface{}, string, io.Reader) error

/**
[dir] -> <path> [not found]  => <path>/
[dir] -> <path> [dir]        => <path>/basename([dir])/
[file] -> <path> [not found] => <path>
[file] -> <path> [dir]       => <path>/basename([file])
*/
func fromRemote(path string, toFunc toFunc, dst interface{}, dstPath string, key_file string) error {
	srcCli, srcPath, err := connectRemote(path, key_file) // connect src sftp
	if err != nil {
		fmt.Printf("failed connect to:" + path)
		return errors.New("failed connect to from sftp")
	}
	defer srcCli.Close()
	sftp := srcCli.Sftp
	dStat := checkDstPath(dst, dstPath)
	if st, err := sftp.Stat(srcPath); err == nil {
		if st.IsDir() {
			if dStat == 1 { // not found
				dstPath = dstPath
			} else if dStat == 2 { // is dir
				dstPath = filepath.Join(dstPath, filepath.Base(srcPath))
			} else {
				return errors.New("dst path invalid")
			}
			// begin upload dir
			walker := sftp.Walk(srcPath)
			plen := len(srcPath)
			for walker.Step() {
				remote_path := filepath.Join(dstPath, walker.Path()[plen:])
				if walker.Stat().IsDir() {
					// os.MkdirAll(local_file, os.FileMode(0700))
					// ignore empty folder
				} else {
					srcFile, err := sftp.Open(walker.Path())
					if err != nil {
						log.Fatal(err)
					}
					defer srcFile.Close()
					err = toFunc(dst, remote_path, srcFile)
					if err != nil {
						return err
					}
				}
			}
		} else {
			if dStat == 3 || dStat == 1 { // file or not found
				dstPath = dstPath
			} else if dStat == 2 { // dir
				dstPath = filepath.Join(dstPath, filepath.Base(srcPath))
			} else {
				return errors.New("dst path invalid")
			}
			srcFile, err := sftp.Open(srcPath)
			if err != nil {
				log.Fatal(err)
			}
			defer srcFile.Close()
			return toFunc(dst, dstPath, srcFile)
		}
	}
	return nil
}
func fromLocal(srcPath string, toFunc toFunc, dst interface{}, dstPath string) error {
	dStat := checkDstPath(dst, dstPath)
	if st, err := os.Stat(srcPath); err == nil {
		if st.IsDir() {
			if dStat == 1 { // not found
				dstPath = dstPath
			} else if dStat == 2 { // is dir
				dstPath = filepath.Join(dstPath, filepath.Base(srcPath))
			} else {
				return errors.New("dst path invalid")
			}
			// begin upload dir
			plen := len(srcPath)
			mywalkfunc := func(path string, info os.FileInfo, err error) error {
				remote_path := filepath.Join(dstPath, path[plen:])
				//log.Printf("walk: %s -> %s  isdir: %v", path, remote_file, info.IsDir())
				if info.IsDir() {
					//c.Sftp.MkdirAll(remote_file)
				} else {
					srcFile, err := os.Open(path)
					if err != nil {
						return err
					}
					defer srcFile.Close()
					return toFunc(dst, remote_path, srcFile)
				}
				return nil
			}
			return filepath.Walk(srcPath, mywalkfunc)
		} else {
			if dStat == 3 || dStat == 1 { // file or not found
				dstPath = dstPath
			} else if dStat == 2 { // dir
				dstPath = filepath.Join(dstPath, filepath.Base(srcPath))
			} else {
				return errors.New("dst path invalid")
			}
			srcFile, err := os.Open(srcPath)
			if err != nil {
				return err
			}
			defer srcFile.Close()
			return toFunc(dst, dstPath, srcFile)
		}
	}
	return nil
}

/**
文件或目录传送客户端, 支持4种模式的传输, 本地文件或sftp服务器的4个组合
  要求sftp服务端支持 _base_ 共有pubkey用户
参数格式:
   sftpcli -k <priv-key-file> [<sftp_addr>@]<src-path> [<sftp_addr>@]<dst-path>
*/
func FileTran(src, dst, keyfile string) error {
	var err error
	if strings.Index(src, "@") > 0 && strings.Index(dst, "@") > 0 {
		// remote to remote
		dstCli, dstPath, err := connectRemote(dst, keyfile) // connect src sftp
		if err != nil {
			fmt.Printf("failed connect to:" + dst)
			return err
		}
		defer dstCli.Close()
		err = fromRemote(src, toRemote, dstCli, dstPath, keyfile)
	} else if strings.Index(src, "@") > 0 {
		// remote to local
		err = fromRemote(src, toLocal, nil, dst, keyfile)
	} else if strings.Index(dst, "@") > 0 {
		// local to remote
		dstCli, dstPath, err := connectRemote(dst, keyfile) // connect src sftp
		if err != nil {
			fmt.Printf("failed connect to:" + dst)
			return err
		}
		defer dstCli.Close()
		err = fromLocal(src, toRemote, dstCli, dstPath)
	} else {
		// local to local
		err = fromLocal(src, toLocal, nil, dst)
	}
	return err
}
func main() {
	key_file := flag.String("k", "", "ssh private key file")
	usage := func() {
		fmt.Println("sftpcli -k <priv-key-file> [<sftp_addr>@]<src-path> [<sftp_addr>@]<dst-path>")
	}

	flag.Parse()
	if flag.NArg() != 2 {
		usage()
		return
	}
	src := flag.Arg(0)
	dst := flag.Arg(1)
	fmt.Println(src, dst, *key_file)
	err := FileTran(src, dst, *key_file)
	if err != nil {
		os.Exit(1)
	}

	//fmt.Printf("finished with %v\n", err)
}
