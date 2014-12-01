package hsync

import (
	"crypto/md5"
	"fmt"
)

func StrMd5(mystr string) string {
	return ByteMd5([]byte(mystr))
}

func ByteMd5(data []byte) string {
	h := md5.New()
	h.Write(data)
	return fmt.Sprintf("%x", h.Sum(nil))
}
