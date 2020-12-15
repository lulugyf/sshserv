package serv

import (
	"github.com/lulugyf/sshserv/logger"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"strconv"
	"sync"
)

///////////////////////// Remote Port Forward  /////////////

type ForwardedTCPHandler struct {
	forwards map[string]net.Listener
	sync.Mutex
}

// ForwardedTCPHandler can be enabled by creating a ForwardedTCPHandler and
// adding the handlePortforward callback to the server's RequestHandlers under
// tcpip-forward and cancel-tcpip-forward.
func (h *ForwardedTCPHandler) handlePortforward(conn *ssh.ServerConn, req *ssh.Request) (bool, []byte, string) {
	h.Lock()
	if h.forwards == nil {
		h.forwards = make(map[string]net.Listener)
	}
	h.Unlock()

	switch req.Type {
	case "tcpip-forward":
		var reqPayload remoteForwardRequest
		if err := ssh.Unmarshal(req.Payload, &reqPayload); err != nil {
			logger.Error(logSender,"R Unmarshal failed %v", err)
			return false, []byte{}, ""
		}
		addr := net.JoinHostPort(reqPayload.BindAddr, strconv.Itoa(int(reqPayload.BindPort)))
		logger.Debug(logLforward, "bind addr: [%s]\n", addr)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			logger.Error(logSender,"R listen failed %v", err)
			return false, []byte{}, ""
		}
		_, destPortStr, _ := net.SplitHostPort(ln.Addr().String())
		destPort, _ := strconv.Atoi(destPortStr)
		h.Lock()
		h.forwards[addr] = ln
		h.Unlock()
		go func() {
			logger.Debug(logSender,"   begin R accept...")
			for {
				c, err := ln.Accept()
				if err != nil {
					logger.Error(logSender,"R accept failed %v", err)
					break
				}
				originAddr, orignPortStr, _ := net.SplitHostPort(c.RemoteAddr().String())
				originPort, _ := strconv.Atoi(orignPortStr)
				payload := ssh.Marshal(&remoteForwardChannelData{
					DestAddr:   reqPayload.BindAddr,
					DestPort:   uint32(destPort),
					OriginAddr: originAddr,
					OriginPort: uint32(originPort),
				})
				go func() {
					ch, reqs, err := conn.OpenChannel("forwarded-tcpip", payload)
					if err != nil {
						logger.Error(logSender, "open forwarded-tcpip channel failed, %v", err)
						c.Close()
						return
					}
					go ssh.DiscardRequests(reqs)
					go func() {
						defer ch.Close()
						defer c.Close()
						io.Copy(ch, c)
					}()
					go func() {
						defer ch.Close()
						defer c.Close()
						io.Copy(c, ch)
					}()
				}()
			}
			h.Lock()
			delete(h.forwards, addr)
			h.Unlock()
		}()
		return true, ssh.Marshal(&remoteForwardSuccess{uint32(destPort)}), addr

	case "cancel-tcpip-forward":
		var reqPayload remoteForwardCancelRequest
		if err := ssh.Unmarshal(req.Payload, &reqPayload); err != nil {
			// TODO: log parse failure
			return false, []byte{}, ""
		}
		addr := net.JoinHostPort(reqPayload.BindAddr, strconv.Itoa(int(reqPayload.BindPort)))
		h.Lock()
		ln, ok := h.forwards[addr]
		h.Unlock()
		if ok {
			ln.Close()
		}
		return true, nil, ""
	default:
		return false, nil, ""
	}
}





///////////////////////// Local Port Forward  /////////////
type remoteForwardRequest struct {
	BindAddr string
	BindPort uint32
}

type remoteForwardSuccess struct {
	BindPort uint32
}

type remoteForwardCancelRequest struct {
	BindAddr string
	BindPort uint32
}

type remoteForwardChannelData struct {
	DestAddr   string
	DestPort   uint32
	OriginAddr string
	OriginPort uint32
}

type localForwardChannelData struct {
	DestAddr string
	DestPort uint32

	OriginAddr string
	OriginPort uint32
}

func HandleDirectTCPIP( conn *ssh.ServerConn, newChan ssh.NewChannel) {
	d := localForwardChannelData{}
	if err := ssh.Unmarshal(newChan.ExtraData(), &d); err != nil {
		newChan.Reject(ssh.ConnectionFailed, "error parsing forward data: "+err.Error())
		return
	}

	dest := net.JoinHostPort(d.DestAddr, strconv.FormatInt(int64(d.DestPort), 10))
	logger.Debug(logLforward, "forward to dest: %s", dest)

	var dialer net.Dialer
	dconn, err := dialer.Dial("tcp", dest)
	if err != nil {
		newChan.Reject(ssh.ConnectionFailed, err.Error())
		return
	}

	ch, reqs, err := newChan.Accept()
	if err != nil {
		dconn.Close()
		return
	}
	go ssh.DiscardRequests(reqs)

	go func() {
		defer ch.Close()
		defer dconn.Close()
		io.Copy(ch, dconn)
	}()
	go func() {
		defer ch.Close()
		defer dconn.Close()
		io.Copy(dconn, ch)
		logger.Debug(logLforward, "forward to [%s] done!!", dest)
	}()
}
