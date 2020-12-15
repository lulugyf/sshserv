package utils

import "github.com/lulugyf/sshserv/logger"

// SetUmask does nothing on windows
func SetUmask(umask int, configValue string) {
	logger.Debug(logSender, "umask not available on windows, configured value %v (%v)", configValue, umask)
}
