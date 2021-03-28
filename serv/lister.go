package serv

import (
	"io"
	"os"
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

	maxlen := offset+int64(len(f))
	if maxlen > int64(len(l)) {
		maxlen = int64(len(l))
	}
	n := 0
	for i, fi := range l[offset:maxlen] {
		f[i] = & sftpFileInfo{fi }
		n += 1
	}
	//n := copy(f, l[offset:])
	if n < len(f) {
		return n, io.EOF
	}
	return n, nil
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
