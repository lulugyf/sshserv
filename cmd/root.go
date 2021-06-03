package cmd

import (
	"fmt"
	"os"

	"github.com/lulugyf/sshserv/utils"
	"github.com/spf13/cobra"
)

const (
	logSender = "cmd"
)

var (
	rootCmd = &cobra.Command{
		Use:   "sshserv",
		Short: "Full featured and highly configurable SSH server",
	}
)

func init() {
	version := utils.GetAppVersion()
	rootCmd.Flags().BoolP("version", "v", false, "")
	rootCmd.Version = version.GetVersionAsString()
	rootCmd.SetVersionTemplate(`{{printf "SSHServ version: "}}{{printf "%s" .Version}}
`)
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
