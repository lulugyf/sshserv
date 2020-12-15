// +build !darwin,!linux,!plan9

package sftp

import (
	"syscall"
)

func (p sshFxpExtendedPacketStatVFS) respond(svr *Server) responsePacket {
	return statusFromError(p, syscall.ENOTSUP)
}
