
sshserv: main.go serv/shell_linux.go
	go build -ldflags="-s -w"

win:
	GOOS=windows GOARCH=386 go build -ldflags="-s -w"

clean:
	rm -f sshserv sshserv.exe cli/cli cli/sftpcli dist/sftpgo.log
