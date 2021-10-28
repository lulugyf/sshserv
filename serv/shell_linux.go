// +build linux

package serv

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/lulugyf/sshserv/dataprovider"
	"github.com/lulugyf/sshserv/logger"
	//"github.com/kr/pty"
	"github.com/creack/pty"
	"github.com/anmitsu/go-shlex"
	"golang.org/x/crypto/ssh"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"
)

var (
	defaultShell = "sh" // Shell used if the SHELL environment variable isn't set
	logShell     = "shell"
)

// Start assigns a pseudo-terminal tty os.File to c.Stdin, c.Stdout,
// and c.Stderr, calls c.Start, and returns the File of the tty's
// corresponding pty.
func PtyRun(c *exec.Cmd, tty *os.File) (err error) {
	defer tty.Close()
	c.Stdout = tty
	c.Stdin = tty
	c.Stderr = tty
	c.SysProcAttr = &syscall.SysProcAttr{
		Setctty: true,
		Setsid:  true,
	}
	return c.Start()
}

// parseDims extracts two uint32s from the provided buffer.
func parseDims(b []byte) (uint32, uint32) {
	w := binary.BigEndian.Uint32(b)
	h := binary.BigEndian.Uint32(b[4:])
	return w, h
}

// Winsize stores the Height and Width of a terminal.
type Winsize struct {
	Height uint16
	Width  uint16
	x      uint16 // unused
	y      uint16 // unused
}

// SetWinsize sets the size of the given pty.
func SetWinsize(fd uintptr, w, h uint32) {
	logger.Debug("", "window resize %dx%d", w, h)
	ws := &Winsize{Width: uint16(w), Height: uint16(h)}
	syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TIOCSWINSZ), uintptr(unsafe.Pointer(ws)))
}

func handleShell(req *ssh.Request, channel ssh.Channel, f, tty *os.File, homedir string) bool{
	// allocate a terminal for this channel
	logger.Debug("shell", "creating pty...")

	var shell string
	shell = os.Getenv("SHELL")
	if shell == "" {
		_, err := os.Stat("/bin/bash")
		if err == nil {
			shell = "/bin/bash"
		}else {
			shell = defaultShell
		}
	}

	cmd := exec.Command(shell)
	cmd.Dir = homedir
	cmd.Env = append(os.Environ(), "TERM=xterm", fmt.Sprintf("HOME=%s", homedir))
	err := PtyRun(cmd, tty)
	if err != nil {
		logger.Warn("", "%s", err)
	}

	// Teardown session
	var once sync.Once
	close := func() {
		channel.Close()
		f.Close()
		tty.Close()
		p_stat, err := cmd.Process.Wait()
		logger.Warn(logShell,"session closed, p_stat: %v err: %v", p_stat, err)
	}

	// Pipe session to bash and visa-versa
	go func() {
		io.Copy(channel, f)
		once.Do(close)
	}()

	go func() {
		io.Copy(f, channel)
		once.Do(close)
	}()

	// We don't accept any commands (Payload),
	// only the default shell.
	if len(req.Payload) == 0 {
		//ok = true
	}
	return true
}

func handlePtrReq(req *ssh.Request) (*os.File, *os.File){
	// Create new pty
	fPty, tty, err := pty.Open()
	if err != nil {
		logger.Warn(logShell, "could not start pty (%s)", err)
		return nil, nil
	}
	// Parse body...
	termLen := req.Payload[3]
	termEnv := string(req.Payload[4 : termLen+4])
	w, h := parseDims(req.Payload[termLen+4:])
	SetWinsize(fPty.Fd(), w, h)
	logger.Debug(logShell, "pty-req '%s'", termEnv)
	return fPty, tty
}


func copyBuffer(dst io.Writer, src io.Reader, buf []byte) (written int64, err error) {
	if buf == nil {
		size := 32 * 1024
		if l, ok := src.(*io.LimitedReader); ok && int64(size) > l.N {
			if l.N < 1 {
				size = 1
			} else {
				size = int(l.N)
			}
		}
		buf = make([]byte, size)
	}
	for {
		nr, er := src.Read(buf)
		fmt.Printf("---- read nbytes: %d, err: %v\n", nr, er)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

func PtyRun1(c *exec.Cmd, tty *os.File) (io.ReadCloser, error) {
	defer tty.Close()
	c.Stdout = tty
	c.Stdin = tty
	c.SysProcAttr = &syscall.SysProcAttr{
		Setctty: true,
		Setsid:  true,
	}
	errReader, err := c.StderrPipe()
	if err != nil {
		return nil, err
	}
	return errReader, c.Start()
}

func handleExec(req *ssh.Request, channel ssh.Channel, cmd *exec.Cmd) error{
	fPty, tty, err := pty.Open()
	if err != nil {
		logger.Warn(logShell, "could not start pty (%s)", err)
		return errors.New("open pty failed")
	}


	err = PtyRun(cmd, tty)
	if err != nil {
		logger.Warn("", "%s", err)
		return err
	}

	// Teardown session
	var once sync.Once
	close := func() {
		//stderr.Close()
		tty.Close()
		fPty.Close()
		channel.Close()
		//if ! cmd.ProcessState.Exited() {
			cmd.Process.Kill()
		//}
		p_stat, err := cmd.Process.Wait()
		logger.Warn(logShell,"exec-- closed, p_stat: %v err: %v", p_stat, err)
	}

	// Pipe session to bash and visa-versa
	go func() {
		nbytes, err := io.Copy(channel, fPty)
		logger.Info(logShell, "exec-- send nbytes %d, err: %v", nbytes, err)
		once.Do(close)
	}()

	go func() {
		nbytes, err := io.Copy(fPty, channel) // copyBuffer(fPty, channel, nil)
		logger.Info(logShell, "exec-- receive nbytes %d, err: %v", nbytes, err)
		once.Do(close)
	}()

	return nil
}

func handleWindowChanged(req *ssh.Request, fPty *os.File) {
	w, h := parseDims(req.Payload)
	SetWinsize(fPty.Fd(), w, h)
}



func handleSSHRequest(in <-chan *ssh.Request, channel ssh.Channel, connection Connection, c *Configuration) {
	var fPty *os.File = nil
	var tty *os.File = nil
	for req := range in {
		ok := false
		logger.Debug(logSender,"--- req.Type: [%s] payload [%s]\n", req.Type, string(req.Payload))

		switch req.Type {
		case "subsystem":
			if string(req.Payload[4:]) == "sftp" {
				ok = true
				connection.protocol = protocolSFTP
				go c.handleSftpConnection(channel, connection)
			}
		case "exec":
			var msg execMsg
			if err := ssh.Unmarshal(req.Payload, &msg); err == nil {
				//name, execArgs, err := parseCommandPayload(msg.Command)
				parts, err := shlex.Split(msg.Command, true)
				name, execArgs := parts[0], parts[1:]
				//fmt.Printf("------exec %s\n", name)
				logger.Debug(logSender, "new exec command: %v args: %v user: %v, error: %v", name, execArgs,
					connection.User.Username, err)
				if c.IsSCPEnabled && err == nil && name == "scp" && len(execArgs) >= 2 {
					ok = true
					connection.protocol = protocolSCP
					scpCommand := scpCommand{
						connection: connection,
						args:       execArgs,
						channel:    channel,
					}
					go scpCommand.handle()
				}else if err == nil {
					// execute cmd
					if connection.User.HasPerm(dataprovider.PermShell) {
						cmd := exec.Command(name, execArgs...)
						cmd.Env = append(os.Environ(), "TERM=vt100",
							fmt.Sprintf("HOME=%s", connection.User.HomeDir))
						cmd.Dir = connection.User.HomeDir
						err = handleExec(req, channel, cmd)
						if err != nil {
							logger.Error(logShell, "exec failed: %v", err)
							ok = false
						}else {
							logger.Info(logShell, "exec started.")
							ok = true // 还是需要关闭连接
						}
						req.Reply(ok, nil)
						return // this will end the session
					}
				}else {
					logger.Error(logShell, "parseCommandPayload failed: %v", err)
				}
			}
		case "shell":
			if fPty == nil {
				logger.Warn(logShell, "pty not open yet!")
				ok = false
			} else {
				ok = handleShell(req, channel, fPty, tty, connection.User.HomeDir)
			}
		case "pty-req":
			if c.FullFunc && connection.User.HasPerm(dataprovider.PermShell) {
				// Responding 'ok' here will let the client
				// know we have a pty ready for input
				ok = true
				fPty, tty = handlePtrReq(req)
				if fPty == nil {
					ok = false
				}
			}else{
				ok = false
				logger.Warn(logShell, "Denied shell of user [%s] full_func:[%v] perms:[%v]",
					connection.User.Username, c.FullFunc, connection.User.Permissions)
			}
		case "window-change":
			if fPty == nil {
				logger.Warn(logShell, "pty not open yet!")
				ok = false
			}else {
				handleWindowChanged(req, fPty)
			}
			continue //no response
		case "env":

		}
		req.Reply(ok, nil)
	}
	logger.Debug(logSender, " --request process exited...")
	if fPty != nil {
		fPty.Close()
		tty.Close()
		logger.Debug(logSender, " --pty closed")
	}
}
