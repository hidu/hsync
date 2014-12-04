package hsync

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"net"
	"net/http"
	"bufio"
	"net/rpc"
	"time"
	"errors"
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


func RpcDialHTTPPath(network, address, path string,timeout time.Duration) (*rpc.Client, error) {
	var err error
	conn, err := net.DialTimeout(network, address,timeout)
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

func checkDir(dir string,mode os.FileMode) error{
	_, err:= os.Stat(dir)
	if os.IsNotExist(err) {
		return os.MkdirAll(dir, mode)
	}
	return err
}

func copyFile(dest,src string)error{
	f,err:=os.Open(src)
	if(err!=nil){
		return err
	}
	defer f.Close()
	info,err:=f.Stat()
	if(err!=nil || info.IsDir()){
		return fmt.Errorf("src is dir")	
	}
	d,err:=os.OpenFile(dest,os.O_RDWR|os.O_CREATE,info.Mode())
	defer d.Close()
	_,err=io.Copy(d,f)
	return err
}