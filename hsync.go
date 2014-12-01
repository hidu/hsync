package main

import (
	"flag"
	//	"github.com/hidu/hsync/hsync"
	"./hsync"
	"github.com/golang/glog"
	"os"
	"path/filepath"
)

var addr = flag.String("addr", ":1234", "")
var root = flag.String("root", "./data/", "")
var d = flag.Bool("d", false, "")

func init() {
	flag.Set("logtostderr", "1")
}

func main() {
	flag.Parse()
	dirAbs, err := filepath.Abs(*root)
	if err != nil {
		glog.Errorln("root dir wrong!", err)
		os.Exit(1)
	}

	os.Chdir(*root)
	if *d {
		server, err := hsync.NewHsyncServer(*addr, dirAbs)
		if err != nil {
			glog.Errorln("start server failed:", err)
			os.Exit(1)
		}
		server.Start()
	} else {
		client, _ := hsync.NewHsyncClient(*addr, dirAbs)
		client.Connect()
		client.Watch()
	}
}
