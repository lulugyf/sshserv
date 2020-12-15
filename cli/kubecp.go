package main

// use to copy file(fold) between kubernetes containers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"github.com/sirupsen/logrus"
	"os/exec"

	"io"
	"os"
	"path/filepath"
	"strings"
	"flag"
)


func test1() {
	path0 := "D:/gyf/bt/music/The Official UK Top 40 Singles Chart (06.11.2020)/10. Justin Bieber - Holy (feat. Chance the Rapper).mp3"
	path1 := strings.ReplaceAll(path0, " ", "%20")
	st, err := os.Stat(path1)

	if err != nil {
		fmt.Printf(" failed %v\n", err)
	}else{
		fmt.Printf("%v\n", st)
	}
}

func main() {
	kubecfg := flag.String("c", "", "kubeconfig file path(optional)")
	flag.Parse()
	if len(flag.Args()) != 2 {
		fmt.Printf("Usage: %s [-c <kubeconfig>] <[src-pod:]src-path> <[dst-pod:]dst-path>\n", os.Args[0])
		return
	}

	src := flag.Arg(0)
	dst := flag.Arg(1)
	if strings.Index(src, ":") > 0 && strings.Index(dst, ":") > 0 {
		ss := strings.SplitN(src, ":", 2)
		srcPod := ss[0]
		srcPath := ss[1]
		ss = strings.SplitN(dst, ":", 2)
		dstPod := ss[0]
		dstPath := ss[1]

		err := copyBetweenContainers(srcPod, srcPath, dstPod, dstPath, *kubecfg)
		fmt.Printf("return %v\n", err)
		//arr := []interface{}{"a", "b"}
		//arr = append(arr, "c")
		//fmt.Printf("%v  %v %v", arr...)
	}else if strings.Index(src, ":") > 0 {
		ss := strings.SplitN(src, ":", 2)
		srcPod := ss[0]
		srcPath := ss[1]
		err := copyFromContainer(srcPod, srcPath, dst, *kubecfg)
		fmt.Printf("return %v\n", err)
	}else if strings.Index(dst, ":" ) > 0 {
		ss := strings.SplitN(dst, ":", 2)
		dstPod := ss[0]
		dstPath := ss[1]
		err := copyToContainer(src, dstPod, dstPath, *kubecfg)
		fmt.Printf("return %v\n", err)
	}else {
		fmt.Printf("Use cp instead please!")
	}

	//fmt.Println("hello")

	//Tar("/tmp/abc", "/tmp/x1")


	// copy directory or file into container

	//copyIntoContainer("/tmp/abc", "mtgw", "/tmp")
	//copyIntoContainer("bolt.go", "mtgw", "/tmp")

	//err := copyFromContainer("mtgw", "/tmp/abc", "/tmp/x1")
	//fmt.Printf("return %v\n", err)

	//copyBetweenContainers("mtgw", "/tmp/abc", "pyspark-676cb8958c-rj7sp", "/tmp")

	// kubectl exec <src-pod> -- tar cmf - -C <src-dir> <file or folder> | kubectl exec -i <dst-pod> -- tar xmf - --no-same-owner -C <dst-dir>
}


func copyBetweenContainers(srcPod, src, dstPod, dst string, kubecfg string) error {
	// https://stackoverflow.com/questions/10781516/how-to-pipe-several-commands-in-go
	src_dir := filepath.Dir(src)
	src_name := filepath.Base(src)
	var c1, c2 *exec.Cmd
	if kubecfg != "" {
		c1 = exec.Command("kubectl", "--kubeconfig", kubecfg, "exec", srcPod,
			"--", "tar", "cmf", "-", "-C", src_dir, src_name)
		c2 = exec.Command("kubectl", "--kubeconfig", kubecfg, "exec", "-i", dstPod,
			"--", "tar", "xmf", "-", "-C", dst, "--no-same-owner")
	}else {
		c1 = exec.Command("kubectl", "exec", srcPod,
			"--", "tar", "cmf", "-", "-C", src_dir, src_name)
		c2 = exec.Command("kubectl", "exec", "-i", dstPod,
			"--", "tar", "xmf", "-", "-C", dst, "--no-same-owner")
	}

	r, w := io.Pipe()
	c1.Stdout = w
	c2.Stdin = r

	var b2 bytes.Buffer
	c2.Stdout = &b2

	c1.Start()
	c2.Start()
	c1.Wait()
	w.Close()
	c2.Wait()
	io.Copy(os.Stdout, &b2)

	return nil
}


func copyFromContainer(podName, src, dst string, kubecfg string) error {
	// kubectl exec -i mtgw -- tar cmf - -C /tmp abc >x
	src_dir := filepath.Dir(src)
	src_name := filepath.Base(src)
	fmt.Println("====", src_dir, src_name)
	reader, writer := io.Pipe()
	var copy *exec.Cmd
	if kubecfg != "" {
		copy = exec.Command("kubectl","--kubeconfig", kubecfg, "exec", podName,
			"--", "tar", "cmf", "-", "-C", src_dir, src_name)
	}else {
		copy = exec.Command("kubectl", "exec", podName,
			"--", "tar", "cmf", "-", "-C", src_dir, src_name)
	}
	copy.Stdout = writer

	copy.Start()
	if err := tarGetFiles(reader, dst); err != nil {
		logrus.Errorln("Error extract tar archive:", err)
	}
	reader.Close()

	if err := copy.Wait(); err != nil {
		//fmt.Printf("--Failed %v\n", err)
		writer.Close()
		copy.Process.Release()
		if strings.Index(err.Error(), "io: read/write on closed pipe") >= 0 {
			logrus.Println("---closed pipe")
			return nil
		}
		return err
	}else{
		fmt.Println("success!")
		return nil
	}
}


func tarGetFiles(reader io.Reader, target string) error {
	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		path := filepath.Join(target, header.Name)
		info := header.FileInfo()
		if info.IsDir() {
			if err = os.MkdirAll(path, info.Mode()); err != nil {
				return err
			}
			continue
		}

		file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(file, tarReader)
		if err != nil {
			return err
		}
	}
	return nil
}

func copyToContainer(src, podName, dst string, kubecfg string) error {
	reader, writer := io.Pipe()
	//copy := exec.Command("kubectl", "exec", "pod.Name", "--namespace", "pod.Namespace", "-c", "container.Name", "-i",
	//	"--", "tar", "xmf", "-", "-C", "/", "--no-same-owner") // pass all the flags you want to
	var copy *exec.Cmd
	if kubecfg != "" {
		copy = exec.Command("kubectl", "--kubeconfig", kubecfg, "exec", podName, "-i",
			"--", "tar", "xmf", "-", "-C", dst, "--no-same-owner")
	}else {
		copy = exec.Command("kubectl", "exec", podName, "-i",
			"--", "tar", "xmf", "-", "-C", dst, "--no-same-owner")
	}
	copy.Stdin = reader
	go func() {
		defer writer.Close()
		if err := tarAddFiles(writer, src); err != nil {
			logrus.Errorln("Error creating tar archive:", err)
		}
	}()

	copy.Start()
	if err := copy.Wait(); err != nil {
		fmt.Printf("Failed %v\n", err)
		return err
	}else{
		fmt.Println("success!")
		return nil
	}
}


func tarAddFiles(w io.Writer, source string) error {
	tarball := tar.NewWriter(w)
	defer tarball.Close()

	info, err := os.Stat(source)
	if err != nil {
		return nil
	}
	var baseDir string
	if info.IsDir() {
		baseDir = filepath.Base(source)
	}

	return filepath.Walk(source,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			header, err := tar.FileInfoHeader(info, info.Name())
			if err != nil {
				return err
			}

			if baseDir != "" {
				header.Name = filepath.Join(baseDir, strings.TrimPrefix(path, source))
			}

			if err := tarball.WriteHeader(header); err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(tarball, file)
			return err
		})

	return nil
}







func Tar(source, target string) error {
	filename := filepath.Base(source)
	target = filepath.Join(target, fmt.Sprintf("%s.tar", filename))
	tarfile, err := os.Create(target)
	if err != nil {
		return err
	}
	defer tarfile.Close()

	tarball := tar.NewWriter(tarfile)
	defer tarball.Close()

	info, err := os.Stat(source)
	if err != nil {
		return nil
	}

	var baseDir string
	if info.IsDir() {
		baseDir = filepath.Base(source)
	}

	return filepath.Walk(source,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			header, err := tar.FileInfoHeader(info, info.Name())
			if err != nil {
				return err
			}

			if baseDir != "" {
				header.Name = filepath.Join(baseDir, strings.TrimPrefix(path, source))
			}

			if err := tarball.WriteHeader(header); err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(tarball, file)
			return err
		})
}

func Untar(tarball, target string) error {
	reader, err := os.Open(tarball)
	if err != nil {
		return err
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		path := filepath.Join(target, header.Name)
		info := header.FileInfo()
		if info.IsDir() {
			if err = os.MkdirAll(path, info.Mode()); err != nil {
				return err
			}
			continue
		}

		file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(file, tarReader)
		if err != nil {
			return err
		}
	}
	return nil
}

func Gzip(source, target string) error {
	reader, err := os.Open(source)
	if err != nil {
		return err
	}

	filename := filepath.Base(source)
	target = filepath.Join(target, fmt.Sprintf("%s.gz", filename))
	writer, err := os.Create(target)
	if err != nil {
		return err
	}
	defer writer.Close()

	archiver := gzip.NewWriter(writer)
	archiver.Name = filename
	defer archiver.Close()

	_, err = io.Copy(archiver, reader)
	return err
}

func UnGzip(source, target string) error {
	reader, err := os.Open(source)
	if err != nil {
		return err
	}
	defer reader.Close()

	archive, err := gzip.NewReader(reader)
	if err != nil {
		return err
	}
	defer archive.Close()

	target = filepath.Join(target, archive.Name)
	writer, err := os.Create(target)
	if err != nil {
		return err
	}
	defer writer.Close()

	_, err = io.Copy(writer, archive)
	return err
}



