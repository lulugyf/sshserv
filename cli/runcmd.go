package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"github.com/creack/pty"
)


func main() {
	outdir := os.Getenv("NNI_OUTPUT_DIR")
	if outdir == ""{
		outdir = "stderr"
	}else {
		outdir = fmt.Sprintf("%s/stderr", outdir)
	}
	f_stderr, err := os.OpenFile(outdir, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("Error opening file: %v", err)
	}
	defer f_stderr.Close()

	cmd := exec.Command(os.Args[1], os.Args[2:]...)
	// fmt.Printf("cmd: [%s]\n", os.Args[1])
	f, err := pty.Start(cmd)
	defer f.Close()
	if err != nil {
		fmt.Printf("failed with %v\n", err)
		panic(err)
	}
	buf := make([]byte, 4096)
	for {
		i, err := f.Read(buf)
		if err != nil {
			break
		}
		f_stderr.Write(buf[:i]);
	}

}


func main2() {
	// Logging capability
	f_stdout, err := os.OpenFile("stdout", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Error opening file: %v", err)
	}
	//wr_stdout := bufio.NewWriterSize(f_stdout, 0)
	defer f_stdout.Close()

	f_stderr, err := os.OpenFile("stderr", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Error opening file: %v", err)
	}
	//wr_stderr := bufio.NewWriterSize(f_stderr, 0)
	defer f_stderr.Close()

	cmd := exec.Command(os.Args[1], os.Args[2:]...)
	fmt.Printf("cmd: [%s]\n", os.Args[1])
	cmd.Stderr = f_stderr
	cmd.Stdout = f_stdout
	err = cmd.Run() //blocks until sub process is complete
	if err != nil {
		fmt.Printf("failed with %v\n", err)
		panic(err)
	}
}



func main1() {
	// Logging capability
	f, err := os.OpenFile("stdout", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Error opening file: %v", err)
	}
	defer f.Close()
	mwriter := io.MultiWriter(f, os.Stdout)
	cmd := exec.Command("python", "runcmd.py")
	cmd.Stderr = mwriter
	cmd.Stdout = mwriter
	err = cmd.Run() //blocks until sub process is complete
	if err != nil {
		panic(err)
	}
}
