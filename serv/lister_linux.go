// +build linux

package serv

import (
	"github.com/lulugyf/sshserv/sftp"
	"os/user"
	"syscall"
	"fmt"
)

/*
 convert Sys() type, so ls can showed properly
   (ls -l)'s return format code position: sftp/request.go: filelist() -> sftp/server_unix.go: runLs()
*/
func (fi sftpFileInfo) Sys() interface{} {
	st := fi.h.Sys().(*syscall.Stat_t)
	uname := "[unknown]"
	gname := "[unknown]"
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