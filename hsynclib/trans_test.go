package hsync

import (
	"testing"
)

func TestFileGetStatSlice(t *testing.T) {
	var stat FileStatSlice
	//	name := "/home/duwei/down/jdk-8u20-linux-x64.rpm"
	name := "./conf.go"
	err := fileGetStatSlice(name, &stat)
	if err != nil {
		t.Error(err)
	}
	if stat.Total != 1 {
		t.Error("part total wrong")
	}
}
