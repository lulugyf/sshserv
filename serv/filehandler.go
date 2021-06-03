package serv

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lulugyf/sshserv/utils"
	"github.com/rs/xid"

	"github.com/lulugyf/sshserv/dataprovider"
	"github.com/lulugyf/sshserv/logger"
	"golang.org/x/crypto/ssh"

	"github.com/lulugyf/sshserv/sftp"
)

// Connection details for an authenticated user
type Connection struct {
	// Unique identifier for the connection
	ID string
	// logged in user's details
	User dataprovider.User
	// client's version string
	ClientVersion string
	// Remote address for this connection
	RemoteAddr net.Addr
	// start time for this connection
	StartTime time.Time
	// last activity for this connection
	lastActivity time.Time
	protocol     string
	lock         *sync.Mutex
	sshConn      *ssh.ServerConn
}

func (c Connection) ActiveTime() {
	c.lastActivity = time.Now()
}
// Fileread creates a reader for a file on the system and returns the reader back.
func (c Connection) Fileread(request *sftp.Request) (io.ReaderAt, error) {
	updateConnectionActivity(c.ID)

	if !c.User.HasPerm(dataprovider.PermDownload) {
		return nil, sftp.ErrSshFxPermissionDenied
	}

	p, err := c.buildPath(request.Filepath)
	if err != nil {
		return nil, sftp.ErrSshFxNoSuchFile
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	st1, err := os.Stat(p)
	if os.IsNotExist(err) {
		return nil, sftp.ErrSshFxNoSuchFile
	}

	file, err := os.Open(p)
	if err != nil {
		logger.Error(logSender, "could not open file \"%v\" for reading: %v", p, err)
		return nil, sftp.ErrSshFxFailure
	}

	request.Attributes().Size = uint64(st1.Size())
	logger.Debug(logSender, "fileread requested for path: \"%v\", user: %v", p, c.User.Username)

	transfer := Transfer{
		file:          file,
		path:          p,
		start:         time.Now(),
		bytesSent:     0,
		bytesReceived: 0,
		user:          c.User,
		connectionID:  c.ID,
		transferType:  transferDownload,
		lastActivity:  time.Now(),
		isNewFile:     false,
		protocol:      c.protocol,
	}
	addTransfer(&transfer)
	return &transfer, nil
}

// Filewrite handles the write actions for a file on the system.
func (c Connection) Filewrite(request *sftp.Request) (io.WriterAt, error) {
	updateConnectionActivity(c.ID)
	if !c.User.HasPerm(dataprovider.PermUpload) {
		return nil, sftp.ErrSshFxPermissionDenied
	}

	p, err := c.buildPath(request.Filepath)
	if err != nil {
		return nil, sftp.ErrSshFxNoSuchFile
	}

	filePath := p
	if uploadMode == uploadModeAtomic {
		filePath = getUploadTempFilePath(p)
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	stat, statErr := os.Stat(p)
	// If the file doesn't exist we need to create it, as well as the directory pathway
	// leading up to where that file will be created.
	if os.IsNotExist(statErr) {
		logger.Debug(logSender, "upload new file to %s, Flags: %x FileMode: %v  stat: %v",
			filePath, request.Flags, request.Attributes().FileMode().String(), request.AttrFlags())
		return c.handleSFTPUploadToNewFile(p, filePath)
	}

	if statErr != nil {
		logger.Error(logSender, "error performing file stat %v: %v", p, statErr)
		return nil, sftp.ErrSshFxFailure
	}

	// This happen if we upload a file that has the same name of an existing directory
	if stat.IsDir() {
		logger.Warn(logSender, "attempted to open a directory for writing to: %v", p)
		return nil, sftp.ErrSshFxOpUnsupported
	}

	return c.handleSFTPUploadToExistingFile(request.Pflags(), p, filePath, stat.Size())
}

// Filecmd hander for basic SFTP system calls related to files, but not anything to do with reading
// or writing to those files.
func (c Connection) Filecmd(request *sftp.Request) error {
	updateConnectionActivity(c.ID)

	p, err := c.buildPath(request.Filepath)
	if err != nil {
		return sftp.ErrSshFxNoSuchFile
	}

	target, err := c.getSFTPCmdTargetPath(request.Target)
	if err != nil {
		return sftp.ErrSshFxOpUnsupported
	}

	logger.Debug(logSender, "new cmd, method: %v user: %v sourcePath: %v, targetPath: %v, fileMod: %v",
		request.Method, c.User.Username, p, target, request.Attributes().FileMode().String())

	switch request.Method {
	case "Setstat":
		return c.handleSFTPSetstat(p, request)
	case "Rename":
		err = c.handleSFTPRename(p, target)
		if err != nil {
			return err
		}

		break
	case "Rmdir":
		return c.handleSFTPRmdir(p)

	case "Mkdir":
		err = c.handleSFTPMkdir(p)
		if err != nil {
			return err
		}

		break
	case "Symlink":
		err = c.handleSFTPSymlink(p, target)
		if err != nil {
			return err
		}

		break
	case "Remove":
		return c.handleSFTPRemove(p)

	default:
		return sftp.ErrSshFxOpUnsupported
	}

	var fileLocation = p
	if target != "" {
		fileLocation = target
	}

	// we return if we remove a file or a dir so source path or target path always exists here
	utils.SetPathPermissions(fileLocation, c.User.GetUID(), c.User.GetGID())

	return sftp.ErrSshFxOk
}

// Filelist is the handler for SFTP filesystem list calls. This will handle calls to list the contents of
// a directory as well as perform file/folder stat calls.
func (c Connection) Filelist(request *sftp.Request) (sftp.ListerAt, error) {
	updateConnectionActivity(c.ID)
	p, err := c.buildPath(request.Filepath)
	if err != nil {
		return nil, sftp.ErrSshFxNoSuchFile
	}

	switch request.Method {
	case "List":
		if !c.User.HasPerm(dataprovider.PermListItems) {
			return nil, sftp.ErrSshFxPermissionDenied
		}

		logger.Debug(logSender, "requested list file for dir: %v user: %v", p, c.User.Username)

		files, err := ioutil.ReadDir(p)
		if err != nil {
			logger.Error(logSender, "error listing directory: %v", err)
			return nil, sftp.ErrSshFxFailure
		}

		return listerAt(files), nil
	case "Stat":
		if !c.User.HasPerm(dataprovider.PermListItems) {
			return nil, sftp.ErrSshFxPermissionDenied
		}

		logger.Debug(logSender, "requested stat for file: %v user: %v", p, c.User.Username)
		s, err := os.Stat(p)
		if os.IsNotExist(err) {
			return nil, sftp.ErrSshFxNoSuchFile
		} else if err != nil {
			logger.Error(logSender, "error running STAT on file: %v", err)
			return nil, sftp.ErrSshFxFailure
		}

		return listerAt([]os.FileInfo{s}), nil
	default:
		return nil, sftp.ErrSshFxOpUnsupported
	}
}

func (c Connection) getSFTPCmdTargetPath(requestTarget string) (string, error) {
	var target string
	// If a target is provided in this request validate that it is going to the correct
	// location for the server. If it is not, return an operation unsupported error. This
	// is maybe not the best error response, but its not wrong either.
	if requestTarget != "" {
		var err error
		target, err = c.buildPath(requestTarget)
		if err != nil {
			return target, sftp.ErrSshFxOpUnsupported
		}
	}
	return target, nil
}

func (c Connection) handleSFTPRename(sourcePath string, targetPath string) error {
	if !c.User.HasPerm(dataprovider.PermRename) {
		return sftp.ErrSshFxPermissionDenied
	}
	if err := os.Rename(sourcePath, targetPath); err != nil {
		logger.Error(logSender, "failed to rename file, source: %v target: %v: %v", sourcePath, targetPath, err)
		return sftp.ErrSshFxFailure
	}
	logger.CommandLog(renameLogSender, sourcePath, targetPath, c.User.Username, c.ID, c.protocol)
	executeAction(operationRename, c.User.Username, sourcePath, targetPath)
	return nil
}

func (c Connection) handleSFTPRmdir(path string) error {
	if !c.User.HasPerm(dataprovider.PermDelete) {
		return sftp.ErrSshFxPermissionDenied
	}

	numFiles, size, fileList, err := utils.ScanDirContents(path)
	if err != nil {
		logger.Error(logSender, "failed to remove directory %v, scanning error: %v", path, err)
		return sftp.ErrSshFxFailure
	}
	if err := os.RemoveAll(path); err != nil {
		logger.Error(logSender, "failed to remove directory %v: %v", path, err)
		return sftp.ErrSshFxFailure
	}

	logger.CommandLog(rmdirLogSender, path, "", c.User.Username, c.ID, c.protocol)
	dataprovider.UpdateUserQuota(dataProvider, c.User, -numFiles, -size, false)
	for _, p := range fileList {
		executeAction(operationDelete, c.User.Username, p, "")
	}
	return sftp.ErrSshFxOk
}

func (c Connection) handleSFTPSymlink(sourcePath string, targetPath string) error {
	if !c.User.HasPerm(dataprovider.PermCreateSymlinks) {
		return sftp.ErrSshFxPermissionDenied
	}
	if err := os.Symlink(sourcePath, targetPath); err != nil {
		logger.Warn(logSender, "failed to create symlink %v -> %v: %v", sourcePath, targetPath, err)
		return sftp.ErrSshFxFailure
	}

	logger.CommandLog(symlinkLogSender, sourcePath, targetPath, c.User.Username, c.ID, c.protocol)
	return nil
}

func (c Connection) handleSFTPMkdir(path string) error {
	if !c.User.HasPerm(dataprovider.PermCreateDirs) {
		return sftp.ErrSshFxPermissionDenied
	}

	if err := c.createMissingDirs(filepath.Join(path, "testfile")); err != nil {
		logger.Error(logSender, "error making missing dir for path %v: %v", path, err)
		return sftp.ErrSshFxFailure
	}
	logger.CommandLog(mkdirLogSender, path, "", c.User.Username, c.ID, c.protocol)
	return nil
}

func (c Connection) handleSFTPRemove(path string) error {
	if !c.User.HasPerm(dataprovider.PermDelete) {
		return sftp.ErrSshFxPermissionDenied
	}

	var size int64
	var fi os.FileInfo
	var err error
	if fi, err = os.Lstat(path); err != nil {
		logger.Error(logSender, "failed to remove a file %v: stat error: %v", path, err)
		return sftp.ErrSshFxFailure
	}
	size = fi.Size()
	if err := os.Remove(path); err != nil {
		logger.Error(logSender, "failed to remove a file/symlink %v: %v", path, err)
		return sftp.ErrSshFxFailure
	}

	logger.CommandLog(removeLogSender, path, "", c.User.Username, c.ID, c.protocol)
	if fi.Mode()&os.ModeSymlink != os.ModeSymlink {
		dataprovider.UpdateUserQuota(dataProvider, c.User, -1, -size, false)
	}
	executeAction(operationDelete, c.User.Username, path, "")

	return sftp.ErrSshFxOk
}

func (c Connection) handleSFTPUploadToNewFile(requestPath, filePath string) (io.WriterAt, error) {
	if !c.hasSpace(true) {
		logger.Info(logSender, "denying file write due to space limit")
		return nil, sftp.ErrSshFxFailure
	}

	if _, err := os.Stat(filepath.Dir(requestPath)); os.IsNotExist(err) {
		if !c.User.HasPerm(dataprovider.PermCreateDirs) {
			return nil, sftp.ErrSshFxPermissionDenied
		}
	}

	err := c.createMissingDirs(requestPath)
	if err != nil {
		logger.Error(logSender, "error making missing dir for path %v: %v", requestPath, err)
		return nil, sftp.ErrSshFxFailure
	}

	file, err := os.Create(filePath)
	if err != nil {
		logger.Error(logSender, "error creating file %v: %v", requestPath, err)
		return nil, sftp.ErrSshFxFailure
	}

	utils.SetPathPermissions(filePath, c.User.GetUID(), c.User.GetGID())

	transfer := Transfer{
		file:          file,
		path:          requestPath,
		start:         time.Now(),
		bytesSent:     0,
		bytesReceived: 0,
		user:          c.User,
		connectionID:  c.ID,
		transferType:  transferUpload,
		lastActivity:  time.Now(),
		isNewFile:     true,
		protocol:      c.protocol,
	}
	addTransfer(&transfer)
	return &transfer, nil
}

func (c Connection) handleSFTPUploadToExistingFile(pflags sftp.FileOpenFlags, requestPath, filePath string,
	fileSize int64) (io.WriterAt, error) {
	var err error
	if !c.hasSpace(false) {
		logger.Info(logSender, "denying file write due to space limit")
		return nil, sftp.ErrSshFxFailure
	}

	osFlags := getOSOpenFlags(pflags)

	if osFlags&os.O_TRUNC == 0 {
		// see https://github.com/pkg/sftp/issues/295
		logger.Info(logSender, "upload resume is not supported, returning error for file: %v user: %v", requestPath,
			c.User.Username)
		return nil, sftp.ErrSshFxOpUnsupported
	}

	if uploadMode == uploadModeAtomic {
		err = os.Rename(requestPath, filePath)
		if err != nil {
			logger.Error(logSender, "error renaming existing file for atomic upload, source: %v, dest: %v, err: %v",
				requestPath, filePath, err)
			return nil, sftp.ErrSshFxFailure
		}
	}
	// we use 0666 so the umask is applied
	file, err := os.OpenFile(filePath, osFlags, 0666)
	if err != nil {
		logger.Error(logSender, "error opening existing file, flags: %v, source: %v, err: %v", pflags, filePath, err)
		return nil, sftp.ErrSshFxFailure
	}

	// FIXME: this need to be changed when we add upload resume support
	// the file is truncated so we need to decrease quota size but not quota files
	dataprovider.UpdateUserQuota(dataProvider, c.User, 0, -fileSize, false)

	utils.SetPathPermissions(filePath, c.User.GetUID(), c.User.GetGID())

	transfer := Transfer{
		file:          file,
		path:          requestPath,
		start:         time.Now(),
		bytesSent:     0,
		bytesReceived: 0,
		user:          c.User,
		connectionID:  c.ID,
		transferType:  transferUpload,
		lastActivity:  time.Now(),
		isNewFile:     false,
		protocol:      c.protocol,
	}
	addTransfer(&transfer)
	return &transfer, nil
}

func (c Connection) handleSFTPSetstat(filePath string, request *sftp.Request) error {
	attrFlags := request.AttrFlags()
	if attrFlags.Permissions {
		fileMode := request.Attributes().FileMode()
		if err := os.Chmod(filePath, fileMode); err != nil {
			logger.Warn(logSender, "failed to chmod path %#v, mode: %v, err: %+v", filePath, fileMode.String(), err)
			return sftp.ErrSshFxFailure
		}
		//logger.CommandLog(, filePath, "", c.User.Username, fileMode.String(), c.ID, c.protocol, -1, -1, "", "", "")
		return nil
	} else if attrFlags.UidGid {
		uid := int(request.Attributes().UID)
		gid := int(request.Attributes().GID)
		if err := os.Chown(filePath, uid, gid); err != nil {
			logger.Warn( logSender, "failed to chown path %#v, uid: %v, gid: %v, err: %+v", filePath, uid, gid, err)
			return sftp.ErrSshFxFailure
		}
		//logger.CommandLog(chownLogSender, filePath, "", c.User.Username, "", c.ID, c.protocol, uid, gid, "", "", "")
		return nil
	} else if attrFlags.Acmodtime {
		accessTime := time.Unix(int64(request.Attributes().Atime), 0)
		modificationTime := time.Unix(int64(request.Attributes().Mtime), 0)
		if err := os.Chtimes(filePath, accessTime, modificationTime); err != nil {
			logger.Warn( logSender, "failed to chtimes for path %#v, access time: %v, modification time: %v, err: %+v",
				filePath, accessTime, modificationTime, err)
			return sftp.ErrSshFxFailure
		}
		//dateFormat := "2006-01-02T15:04:05" // YYYY-MM-DDTHH:MM:SS
		//accessTimeString := accessTime.Format(dateFormat)
		//modificationTimeString := modificationTime.Format(dateFormat)
		//logger.CommandLog(chtimesLogSender, filePath, "", c.User.Username, "", c.ID, c.protocol, -1, -1, accessTimeString,
		//	modificationTimeString, "")
		return nil
	}
	return nil
}

func (c Connection) hasSpace(checkFiles bool) bool {
	if (checkFiles && c.User.QuotaFiles > 0) || c.User.QuotaSize > 0 {
		numFile, size, err := dataprovider.GetUsedQuota(dataProvider, c.User.Username)
		if err != nil {
			if _, ok := err.(*dataprovider.MethodDisabledError); ok {
				logger.Warn(logSender, "quota enforcement not possible for user %v: %v", c.User.Username, err)
				return true
			}
			logger.Warn(logSender, "error getting used quota for %v: %v", c.User.Username, err)
			return false
		}
		if (checkFiles && c.User.QuotaFiles > 0 && numFile >= c.User.QuotaFiles) ||
			(c.User.QuotaSize > 0 && size >= c.User.QuotaSize) {
			logger.Debug(logSender, "quota exceed for user %v, num files: %v/%v, size: %v/%v check files: %v",
				c.User.Username, numFile, c.User.QuotaFiles, size, c.User.QuotaSize, checkFiles)
			return false
		}
	}
	return true
}

// Normalizes a directory we get from the SFTP request to ensure the user is not able to escape
// from their data directory. After normalization if the directory is still within their home
// path it is returned. If they managed to "escape" an error will be returned.
func (c Connection) buildPath(rawPath string) (string, error) {
	r := filepath.Clean(filepath.Join(c.User.HomeDir, rawPath))
	p, err := filepath.EvalSymlinks(r)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	} else if os.IsNotExist(err) {
		// The requested directory doesn't exist, so at this point we need to iterate up the
		// path chain until we hit a directory that _does_ exist and can be validated.
		_, err = c.findFirstExistingDir(r)
		if err != nil {
			logger.Warn(logSender, "error resolving not existent path: %v", err)
		}
		return r, err
	}

	err = c.isSubDir(p)
	if err != nil {
		logger.Warn(logSender, "Invalid path resolution, dir: %v outside user home: %v err: %v", p, c.User.HomeDir, err)
	}
	return r, err
}

// iterate up the path chain until we hit a directory that does exist and can be validated.
// all nonexistent directories will be returned
func (c Connection) findNonexistentDirs(path string) ([]string, error) {
	results := []string{}
	cleanPath := filepath.Clean(path)
	parent := filepath.Dir(cleanPath)
	_, err := os.Stat(parent)

	for os.IsNotExist(err) {
		results = append(results, parent)
		parent = filepath.Dir(parent)
		_, err = os.Stat(parent)
	}
	if err != nil {
		return results, err
	}
	p, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return results, err
	}
	err = c.isSubDir(p)
	if err != nil {
		logger.Warn(logSender, "Error finding non existing dir: %v", err)
	}
	return results, err
}

// iterate up the path chain until we hit a directory that does exist and can be validated.
func (c Connection) findFirstExistingDir(path string) (string, error) {
	results, err := c.findNonexistentDirs(path)
	if err != nil {
		logger.Warn(logSender, "unable to find non existent dirs: %v", err)
		return "", err
	}
	var parent string
	if len(results) > 0 {
		lastMissingDir := results[len(results)-1]
		parent = filepath.Dir(lastMissingDir)
	} else {
		parent = c.User.GetHomeDir()
	}
	p, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", err
	}
	fileInfo, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if !fileInfo.IsDir() {
		return "", fmt.Errorf("resolved path is not a dir: %v", p)
	}
	err = c.isSubDir(p)
	return p, err
}

// checks if sub is a subpath of the user home dir.
// EvalSymlink must be used on sub before calling this method
func (c Connection) isSubDir(sub string) error {
	// home dir must exist and it is already a validated absolute path
	parent, err := filepath.EvalSymlinks(c.User.HomeDir)
	if err != nil {
		logger.Warn(logSender, "invalid home dir %v: %v", c.User.HomeDir, err)
		return err
	}
	if !strings.HasPrefix(sub, parent) {
		logger.Warn(logSender, "dir %v is not inside: %v ", sub, parent)
		return fmt.Errorf("dir %v is not inside: %v", sub, parent)
	}
	return nil
}

func (c Connection) createMissingDirs(filePath string) error {
	dirsToCreate, err := c.findNonexistentDirs(filePath)
	if err != nil {
		return err
	}
	last := len(dirsToCreate) - 1
	for i := range dirsToCreate {
		d := dirsToCreate[last-i]
		if err := os.Mkdir(d, 0777); err != nil {
			logger.Error(logSender, "error creating missing dir: %v", d)
			return err
		}
		utils.SetPathPermissions(d, c.User.GetUID(), c.User.GetGID())
	}
	return nil
}

func getOSOpenFlags(requestFlags sftp.FileOpenFlags) (flags int) {
	var osFlags int
	if requestFlags.Read && requestFlags.Write {
		osFlags |= os.O_RDWR
	} else if requestFlags.Write {
		osFlags |= os.O_WRONLY
	}
	if requestFlags.Append {
		osFlags |= os.O_APPEND
	}
	if requestFlags.Creat {
		osFlags |= os.O_CREATE
	}
	if requestFlags.Trunc {
		osFlags |= os.O_TRUNC
	}
	if requestFlags.Excl {
		osFlags |= os.O_EXCL
	}
	return osFlags
}

func getUploadTempFilePath(path string) string {
	dir := filepath.Dir(path)
	guid := xid.New().String()
	return filepath.Join(dir, ".sftpgo-upload."+guid+"."+filepath.Base(path))
}
