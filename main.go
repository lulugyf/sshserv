// Full featured and highly configurable SFTP server.
// For more details about features, installation, configuration and usage please refer to the README inside the source tree:
// https://github.com/drakkan/   sftpgo/blob/master/README.md
package main

// done hdfs hostname -> ip
// done sftp + hdfs: ls -l show username and groupname
// done sftp + disk: ls -l show username groupname

/*
alias ss="../sshserv serve &"
alias cc="sftp -P 2022 caro@localhost"
 */

import (
	"github.com/lulugyf/sshserv/cmd"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
//	_ "github.com/colinmarc/hdfs"
)

func main() {
	cmd.Execute()
}
