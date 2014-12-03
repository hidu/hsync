package hsync

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
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
