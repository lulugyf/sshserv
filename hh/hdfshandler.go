package hh

import (
	"errors"
	"fmt"
	"context"
	"net"

	//"github.com/colinmarc/hdfs/protocol/hadoop_hdfs"
	"github.com/lulugyf/sshserv/hdfs"
	"github.com/lulugyf/sshserv/hdfs/hadoopconf"
	"github.com/lulugyf/sshserv/hdfs/intrnl/protocol/hadoop_hdfs"
	"github.com/lulugyf/sshserv/dataprovider"
	"github.com/lulugyf/sshserv/logger"
	"github.com/lulugyf/sshserv/sftp"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	logHDFS = "loghdfs"
	transferDownload = 1
	transferUpload = 2
)
type MyConn interface {
	ActiveTime()
	//Fileread(*Request) (io.ReaderAt, error)
}

type HConnection struct {
	lastActivity time.Time
	protocol     string
	client *hdfs.Client
	User  dataprovider.User
	ConnID string
	conn MyConn
	hosts map[string]string
}
type HTransfer struct {
	rfile         *hdfs.FileReader
	wfile         *hdfs.FileWriter
	path          string
	start         time.Time
	bytesSent     int64
	bytesReceived int64
	user          dataprovider.User
	connectionID  string
	transferType  int
	lastActivity  time.Time
	isNewFile     bool
	protocol      string
}

// ReadAt reads len(p) bytes from the File to download starting at byte offset off and updates the bytes sent.
// It handles download bandwidth throttling too
func (t *HTransfer) ReadAt(p []byte, off int64) (n int, err error) {
	t.lastActivity = time.Now()
	if t.rfile != nil {
		readed, e := t.rfile.ReadAt(p, off)
		t.bytesSent += int64(readed)
		return readed, e
	}
	return -1, errors.New("file not open")
}

// WriteAt writes len(p) bytes to the uploaded file starting at byte offset off and updates the bytes received.
// It handles upload bandwidth throttling too
func (t *HTransfer) WriteAt(p []byte, off int64) (n int, err error) {
	t.lastActivity = time.Now()
	if t.wfile != nil {
		written, e := t.wfile.Write(p)  // can not move cursor
		t.bytesReceived += int64(written)
		return written, e
	}
	return -1, errors.New("file not open")
}

// Close it is called when the transfer is completed.
// It closes the underlying file, log the transfer info, update the user quota, for uploads, and execute any defined actions.
func (t *HTransfer) Close() error {
	var err error
	if t.rfile != nil {
		err = t.rfile.Close()
		t.rfile = nil
	}
	if t.wfile != nil {
		err = t.wfile.Close()
		t.wfile = nil
	}
	return err
}


///////////////////////////////
func NewHandler(conn MyConn, user dataprovider.User, connID string, protocol string, ) *HConnection {
	return & HConnection{
		lastActivity: time.Now(),
		protocol: protocol,
		User: user,
		ConnID: connID,
		conn: conn,
		hosts: make(map[string]string),
	}
}
func (h HConnection) buildPath(rawPath string) (string, error) {
	r := filepath.Join(h.User.HomeDir, rawPath)
	return r, nil
}

func (h HConnection) Fileread(request *sftp.Request) (io.ReaderAt, error) {
	h.conn.ActiveTime()

	if !h.User.HasPerm(dataprovider.PermDownload) {
		return nil, sftp.ErrSshFxPermissionDenied
	}

	p, err := h.buildPath(request.Filepath)
	if err != nil {
		return nil, sftp.ErrSshFxNoSuchFile
	}

	file, err := h.client.Open(p)
	if err != nil {
		logger.Error(logHDFS, "could not open file \"%v\" for reading: %v", p, err)
		return nil, sftp.ErrSshFxFailure
	}
	return &HTransfer{
		rfile:file,
		path:          p,
		start:         time.Now(),
		bytesSent:     0,
		bytesReceived: 0,
		user:          h.User,
		connectionID:  h.ConnID,
		transferType:  transferDownload,
		lastActivity:  time.Now(),
		isNewFile:     false,
		protocol:      h.protocol,
	}, nil
}

func (h HConnection) Filewrite(request *sftp.Request) (io.WriterAt, error) {
	h.conn.ActiveTime()
	if !h.User.HasPerm(dataprovider.PermUpload) {
		return nil, sftp.ErrSshFxPermissionDenied
	}

	p, err := h.buildPath(request.Filepath)
	if err != nil {
		return nil, sftp.ErrSshFxNoSuchFile
	}
	cc := h.client
	if _, err = cc.Stat(p); err == nil {
		// file exists, delete it first
		err = cc.Remove(p)
		if err != nil {
			return nil, sftp.ErrSSHFxFailure
		}
	}
	file, err := cc.Create(p)
	if err != nil {
		return nil, sftp.ErrSSHFxFailure
	}
	return &HTransfer{
		wfile: file,
		path:          p,
		start:         time.Now(),
		bytesSent:     0,
		bytesReceived: 0,
		user:          h.User,
		connectionID:  h.ConnID,
		transferType:  transferUpload,
		lastActivity:  time.Now(),
		isNewFile:     true,
		protocol:      h.protocol,
	}, nil
}

func (h HConnection) getSFTPCmdTargetPath(requestTarget string) (string, error) {
	var target string
	if requestTarget != "" {
		var err error
		target, err = h.buildPath(requestTarget)
		if err != nil {
			return target, sftp.ErrSshFxOpUnsupported
		}
	}
	return target, nil
}

func (h HConnection) Filecmd(request *sftp.Request) error {
	h.conn.ActiveTime()

	p, err := h.buildPath(request.Filepath)
	if err != nil {
		return sftp.ErrSshFxNoSuchFile
	}

	target, err := h.getSFTPCmdTargetPath(request.Target)
	if err != nil {
		return sftp.ErrSshFxOpUnsupported
	}

	logger.Debug(logHDFS, "new cmd, method: %v user: %v sourcePath: %v, targetPath: %v, fileMod: %v",
		request.Method, h.User.Username, p, target, request.Attributes().FileMode().String())

	switch request.Method {
	case "Rename":
		if !h.User.HasPerm(dataprovider.PermRename) {
			return sftp.ErrSshFxPermissionDenied
		}
		err = h.client.Rename(p, target)
		if err != nil {
			return err
		}
	case "Rmdir":
		if !h.User.HasPerm(dataprovider.PermDelete) {
			return sftp.ErrSshFxPermissionDenied
		}
		err = h.client.Remove(p)
		if err != nil {
			return err
		}
	case "Mkdir":
		if !h.User.HasPerm(dataprovider.PermCreateDirs) {
			return sftp.ErrSshFxPermissionDenied
		}
		err = h.client.Mkdir(p, 0755)
		if err != nil {
			return err
		}
	case "Remove":
		err = h.client.Remove(p)
		if err != nil {
			return err
		}
	default:
		return sftp.ErrSshFxOpUnsupported
	}
	return sftp.ErrSshFxOk
}

type listerAt []os.FileInfo
type sftpFileInfo struct {
	h *hdfs.FileInfo
}

/*
 convert Sys() type, so ls can showed properly
   (ls -l)'s return format code position: github.com/pkg/sftp/request.go: filelist() -> server_unix.go: runLs()
 */
func (fi sftpFileInfo) Sys() interface{} {
	st := fi.h.Sys().(*hadoop_hdfs.HdfsFileStatusProto)
	//return &syscall.Stat_t{
	//	Uid: 1000,
	//	Gid: 1000,
	//	Size: int64(st.GetLength()),
	//	Blksize: int64(st.GetBlocksize()),
	//	Mtim: syscall.Timespec{Sec: int64(st.GetModificationTime())/1000},
	//	Mode: *st.GetPermission().Perm,
	//	Nlink: 1,
	//}
	return &sftp.SftpFileAttr{
		Uid: 1000,
		Gid: 1000,
		Nlink: 1,
		Uname: st.GetOwner(),
		Gname: st.GetGroup(),
	}
}

func (fi sftpFileInfo)Name() string {
	return fi.h.Name()
}
func (fi sftpFileInfo)Size() int64 {
	return fi.h.Size()
}
func (fi sftpFileInfo)Mode() os.FileMode {
	return fi.h.Mode()
}
func (fi sftpFileInfo)ModTime() time.Time {
	return fi.h.ModTime()
}
func (fi sftpFileInfo)IsDir() bool {
	return fi.h.IsDir()
}

// ListAt returns the number of entries copied and an io.EOF error if we made it to the end of the file list.
// Take a look at the pkg/sftp godoc for more information about how this function should work.
func (l listerAt) ListAt(f []os.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(l)) {
		return 0, io.EOF
	}

	//fmt.Printf("--- listAt: local len: %d, needlen: %d offset: %d\n", len(l), len(f), offset)
	n := 0
	for i, fi := range l[offset:] {
		f[i] = & sftpFileInfo{fi.(*hdfs.FileInfo) }
		n += 1
	}
	//n := copy(f, l[offset:])
	//x := len(l)-int(offset)
	//for _, fi := range f[:x] {
	//	fmt.Printf("--fi: %s %v %v %v  <mode>: %s\n", fi.Name(), fi.IsDir(), fi.Size(), fi.ModTime(), fi.Mode().String())
	//}
	if n < len(f) {
		return n, io.EOF
	}
	return n, nil
}

func (h HConnection) Filelist(request *sftp.Request) (sftp.ListerAt, error) {
	h.conn.ActiveTime()
	p, err := h.buildPath(request.Filepath)
	if err != nil {
		return nil, sftp.ErrSshFxNoSuchFile
	}

	switch request.Method {
	case "List":
		if !h.User.HasPerm(dataprovider.PermListItems) {
			return nil, sftp.ErrSshFxPermissionDenied
		}
		files, err := h.client.ReadDir(p)
		if err != nil {
			logger.Error(logHDFS, "error listing directory: %v", err)
			return nil, sftp.ErrSshFxFailure
		}
		//fmt.Printf("----list size: %d\n", len(files))
		return listerAt(files), nil
	case "Stat":
		if !h.User.HasPerm(dataprovider.PermListItems) {
			return nil, sftp.ErrSshFxPermissionDenied
		}
		s, err := h.client.Stat(p)
		if os.IsNotExist(err) {
			return nil, sftp.ErrSshFxNoSuchFile
		} else if err != nil {
			logger.Error(logHDFS, "error running STAT on file: %v", err)
			return nil, sftp.ErrSshFxFailure
		}
		//fmt.Printf("----file stat: %s\n", p)
		return listerAt([]os.FileInfo{s}), nil
	default:
		return nil, sftp.ErrSshFxOpUnsupported
	}
}

func _resolveVar(v string, conf map[string]string) string {
	for {
		if strings.Index(v, "${") < 0 {
			return v
		}
		vname := v[strings.Index(v, "${")+2 : strings.Index(v, "}")]
		vval := conf[vname]
		v = strings.ReplaceAll(v, fmt.Sprintf("${%s}", vname), vval)
	}
	return v
}
func resolveVar(namenodes []string, conf map[string]string) []string {
	r := make([]string, len(namenodes))
	for i, v := range namenodes {
		r[i] = _resolveVar(v, conf)
	}
	return r
}


func (h *HConnection)myDialer(ctx context.Context, network, addr string) (net.Conn, error) {
	addr1 := addr
	if len(addr) > 0 && ( addr[0] > '9' || addr[0] < '0' ) {
		i := strings.SplitN(addr, ":", 2)
		addr1 = fmt.Sprintf("%s:%s", h.hosts[i[0]], i[1])
	}
	return (&net.Dialer{}).DialContext(ctx, network, addr1)
}

func (h *HConnection)MkHdfsClient(conf_str string, user string, hosts string) error{
	var namenodes []string
	if _, err := os.Stat(conf_str); os.IsNotExist(err) {
		// not a file, presume it is a namenode string
		namenodes = []string{conf_str}

	}else {
		hadoopConf, err := hadoopconf.Load(conf_str)
		if err != nil {
			fmt.Errorf("can not load hadoop conf")
			return err
		}
		namenodes = hadoopConf.Namenodes()
		namenodes = resolveVar(namenodes, hadoopConf)
	}
	options := hdfs.ClientOptions{
		Addresses: namenodes,
		User: user,
		NamenodeDialFunc: h.myDialer,
		DatanodeDialFunc: h.myDialer,
	}

	if hosts != "" {
		// parse hosts list to map
		for _, i := range strings.Split(hosts, " ") {
			j := strings.Split(i, ",")
			h.hosts[j[0]] = j[1]
		}
	}

	client, err := hdfs.NewClient(options)
	if err != nil {
		return err
	}
	h.client = client

	return nil
}

func (h *HConnection)Close() {
	if h.client != nil {
		h.client.Close()
		h.client = nil
	}
}
func (h *HConnection)FileRead(fpath string ) string{
	//client, _ := hdfs.New("namenode:8020")

	file, _ := h.client.Open(fpath)

	buf := make([]byte, 200)
	file.ReadAt(buf, 10847)

	return string(buf)
}
