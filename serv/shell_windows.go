// +build windows
package serv

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/lulugyf/sshserv/dataprovider"
	"github.com/lulugyf/sshserv/logger"
	winpty "github.com/iamacarpet/go-winpty"
	"golang.org/x/crypto/ssh"
	"io"
	"os/exec"
	"sync"
)

var (
	defaultShell = "cmd" // Shell used if the SHELL environment variable isn't set
	logShell     = "shellw"
)

func handleShell(req *ssh.Request, channel ssh.Channel, pty *winpty.WinPTY) bool{
	// Teardown session
	var once sync.Once
	close := func() {
		channel.Close()
		pty.Close()
		logger.Warn(logShell,"session closed")
	}

	// Pipe session to bash and visa-versa
	go func() {
		io.Copy(channel, pty.StdOut)
		once.Do(close)
	}()

	go func() {
		io.Copy(pty.StdIn, channel)
		once.Do(close)
	}()

	// We don't accept any commands (Payload),
	// only the default shell.
	if len(req.Payload) == 0 {
		//ok = true
	}
	return true
}

func parseDims(b []byte) (uint32, uint32) {
	w := binary.BigEndian.Uint32(b)
	h := binary.BigEndian.Uint32(b[4:])
	return w, h
}
func handlePtrReq(req *ssh.Request, wd string, perms []string) (*winpty.WinPTY){
	//pty, err := winpty.Open("", defaultShell)
	pty, err := winpty.OpenWithOptions(winpty.Options{
		DLLPrefix: "",
		Command:   defaultShell,
		Dir: wd,
	})
	if err != nil {
		logger.Error("Failed to start command: %s\n", err.Error())
	}
	//Set the size of the pty
	termLen := req.Payload[3]
	termEnv := string(req.Payload[4 : termLen+4])
	w, h := parseDims(req.Payload[termLen+4:])
	//SetWinsize(fPty.Fd(), w, h)
	logger.Debug(logShell, "pty-req '%s'", termEnv)
	//pty.SetSize(200, 60)
	pty.SetSize(w, h)
	for _, v := range perms { // Run a initial script for windows
		if len(v) > 5 && v[:5] == "EXEC " {
			pty.StdIn.WriteString(v[5:])
			pty.StdIn.WriteString("\r\n")
			fmt.Printf("pre exec %s\n", v)
		}
	}

	return pty
}
func handleWindowChanged(req *ssh.Request, pty *winpty.WinPTY) {
	w, h := parseDims(req.Payload)
	pty.SetSize(w, h)
}


func handleSSHRequest(in <-chan *ssh.Request, channel ssh.Channel, connection Connection, c *Configuration) {
	var pty *winpty.WinPTY = nil
	var payload_return []byte = nil
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
				name, execArgs, err := parseCommandPayload(msg.Command)
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
						var outbuf, errbuf bytes.Buffer
						cmd.Stdout = &outbuf
						cmd.Stderr = &errbuf
						err = cmd.Run()
						if err != nil {
							logger.Error(logShell, "--exec failed: %v", err)
						}else{
							//fmt.Printf("  output: %s\n", string(out))
							//channel.Write(out)
							//channel.CloseWrite()
							//ok = true  // 还是需要关闭连接
						}
						channel.Write(errbuf.Bytes())
						channel.Write(outbuf.Bytes())
						channel.CloseWrite()
						ok = true  // 还是需要关闭连接
					}
				}else {
					logger.Error(logShell, "parseCommandPayload failed: %v", err)
				}
			}
		case "pty-req":
			if connection.User.HasPerm(dataprovider.PermShell) {
				// Responding 'ok' here will let the client
				// know we have a pty ready for input
				ok = true
				pty = handlePtrReq(req, connection.User.HomeDir, connection.User.Permissions)
				if pty == nil {
					ok = false
				}
			}else{
				ok = false
				logger.Warn(logShell, "Denied shell of user [%s]\n", connection.User.Username)
			}
		case "shell":
			if pty == nil {
				logger.Warn(logShell, "pty not open yet!")
				ok = false
			} else {
				ok = handleShell(req, channel, pty)
			}
		case "window-change":
			if pty == nil {
				logger.Warn(logShell, "pty not open yet!")
				ok = false
			}else {
				handleWindowChanged(req, pty)
			}
			continue //no response
		case "env":

		}
		req.Reply(ok, payload_return)
	}
	logger.Debug(logSender, " --request process exited...")
}
