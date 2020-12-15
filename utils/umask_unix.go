// +build !windows

package utils

import (
	"syscall"

	"github.com/lulugyf/sshserv/logger"
)

// SetUmask sets umask on unix systems
func SetUmask(umask int, configValue string) {
	logger.Debug(logSender, "set umask to %v (%v)", configValue, umask)
	syscall.Umask(umask)
}
