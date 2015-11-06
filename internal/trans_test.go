package internal

import (
	"testing"
)

func TestFileGetStatSlice(t *testing.T) {
	var stat FileStatSlice
	name := "./conf.go"
	err := fileGetStatSlice(name, &stat)
	if err != nil {
		t.Error(err)
	}
	if stat.Total != 1 {
		t.Error("part total wrong")
	}
}
