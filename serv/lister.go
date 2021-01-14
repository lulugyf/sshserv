package serv

import (
	"fmt"
	"github.com/lulugyf/sshserv/sftp"
	"io"
	"os"
	"syscall"
	"os/user"
	"time"
)

type listerAt []os.FileInfo

type sftpFileInfo struct {
	h os.FileInfo
}

// ListAt returns the number of entries copied and an io.EOF error if we made it to the end of the file list.
// Take a look at the pkg/sftp godoc for more information about how this function should work.
func (l listerAt) ListAt(f []os.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(l)) {
		return 0, io.EOF
	}

	n := 0
	for i, fi := range l[offset:] {
		f[i] = & sftpFileInfo{fi }
		n += 1
	}
	//n := copy(f, l[offset:])
	if n < len(f) {
		return n, io.EOF
	}
	return n, nil
}




/*
 convert Sys() type, so ls can showed properly
   (ls -l)'s return format code position: sftp/request.go: filelist() -> sftp/server_unix.go: runLs()
*/
func (fi sftpFileInfo) Sys() interface{} {
	st := fi.h.Sys().(*syscall.Stat_t)
	uname := ""
	gname := ""
	u, err := user.LookupId(fmt.Sprintf("%d", st.Uid) )
	if err == nil {
		uname = u.Username
	}
	g, err := user.LookupGroupId(fmt.Sprintf("%d", st.Gid))
	if err == nil {
		gname = g.Name
	}
	return &sftp.SftpFileAttr{
		Uid: st.Uid,
		Gid: st.Gid,
		Nlink: st.Nlink,
		Uname: uname, //st.GetOwner(),
		Gname: gname, // st.GetGroup(),
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
