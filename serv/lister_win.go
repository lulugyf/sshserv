// +build windows

package serv

import (
	"github.com/lulugyf/sshserv/sftp"
)

/*
 convert Sys() type, so ls can showed properly
   (ls -l)'s return format code position: sftp/request.go: filelist() -> sftp/server_unix.go: runLs()
*/
func (fi *sftpFileInfo) Sys() interface{} {
	return &sftp.SftpFileAttr{
		Uid: 0,
		Gid: 0,
		Nlink: 0,
		Uname: "user", //st.GetOwner(),
		Gname: "group", // st.GetGroup(),
	}
}