package internal

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
)

func StrMd5(mystr string) string {
	return ByteMd5([]byte(mystr))
}

func ByteMd5(data []byte) string {
	h := md5.New()
	h.Write(data)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func FileMd5(name string) string {
	f, err := os.Open(name)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := md5.New()
	io.Copy(h, f)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func RpcDialHTTPPath(network, address, path string, timeout time.Duration) (*rpc.Client, error) {
	var err error
	conn, err := net.DialTimeout(network, address, timeout)
	if err != nil {
		return nil, err
	}
	io.WriteString(conn, "CONNECT "+path+" HTTP/1.0\n\n")

	// Require successful HTTP response
	// before switching to RPC protocol.
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	connected := "200 Connected to Go RPC"

	if err == nil && resp.Status == connected {
		return rpc.NewClient(conn), nil
	}
	if err == nil {
		err = errors.New("unexpected HTTP response: " + resp.Status)
	}
	conn.Close()
	return nil, &net.OpError{
		Op:   "dial-http",
		Net:  network + " " + address,
		Addr: nil,
		Err:  err,
	}
}

func checkDir(dir string, mode os.FileMode) error {
	_, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	return err
}

var _copyrw sync.Mutex

func copyFile(dest, src string) (err error) {
	glog.V(2).Infof("copyFile [%s] -> [%s]", src, dest)
	if glog.V(2) {
		defer func() {
			glog.Warningln("copyFile ", src, "->", dest, "err=", err)
		}()
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.IsDir() {
		_, err := os.Stat(dest)
		if os.IsNotExist(err) {
			err = os.MkdirAll(dest, info.Mode())
			if err != nil {
				return err
			}
		}
		err = filepath.Walk(src, func(fileName string, info os.FileInfo, err error) error {
			if !info.IsDir() {
				rel, _ := filepath.Rel(src, fileName)

				pathDest := filepath.Join(dest, rel)
				return copyFile(pathDest, fileName)
			}
			return nil
		})
	} else {
		_, err = os.Stat(filepath.Dir(src))
		if err != nil {
			return err
		}
		destDir := filepath.Dir(dest)
		if _, err := os.Stat(destDir); os.IsNotExist(err) {
			err = checkDir(destDir, 0755)
			if err != nil {
				return err
			}
		}
		if !info.Mode().IsDir() {
			glog.Infof("copyFile src [%s] is not dir,dest [%s] removeAll", src, dest)
			os.RemoveAll(dest)
		}
		_copyrw.Lock()
		defer _copyrw.Unlock()
		var d *os.File
		d, err = os.OpenFile(dest, os.O_RDWR|os.O_CREATE, info.Mode())
		defer d.Close()
		d.Truncate(0)
		_, err = io.Copy(d, f)
	}
	return err
}

func dataGzipEncode(data []byte) (out []byte) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write(data)
	gw.Flush()
	gw.Close()
	return buf.Bytes()
}

func dataGzipDecode(data []byte) (out []byte) {
	gr, _ := gzip.NewReader(bytes.NewBuffer(data))
	bs, _ := ioutil.ReadAll(gr)
	return bs
}

// loadJsonFile load json
func loadJSONFile(jsonPath string, val interface{}) error {
	bs, err := ioutil.ReadFile(jsonPath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(bs), "\n")
	var bf bytes.Buffer
	for _, line := range lines {
		lineNew := strings.TrimSpace(line)
		if (len(lineNew) > 0 && lineNew[0] == '#') || (len(lineNew) > 1 && lineNew[0:2] == "//") {
			continue
		}
		bf.WriteString(lineNew)
	}
	return json.Unmarshal(bf.Bytes(), &val)
}
