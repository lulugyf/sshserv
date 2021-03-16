// +build windows

package sftp


type SftpFileAttr struct {
	Nlink uint64
	Uid uint32
	Gid uint32
	Uname string
	Gname string
}

