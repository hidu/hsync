/**
* sync dir
* author: hidu <duv123+git@gmail.com>
* https://github.com/hidu/hsync
 */

package main

import (
	"flag"
	"github.com/hidu/hsync/hsync"
	//	"./hsync"
	"fmt"
	"github.com/golang/glog"
	"os"
	"path/filepath"
)

var addr = flag.String("addr", ":8500", "server listen addr")
var home = flag.String("home", "./data/", "dir to sync")
var d = flag.Bool("d", false, "run model,server | client")

func init() {
	flag.Set("logtostderr", "1")

	df := flag.Usage
	flag.Usage = func() {
		df()
		fmt.Fprintln(os.Stderr, "\n  hsync is tool for dir sync, https://github.com/hidu/hsync/")
	}
}

func main() {
	flag.Parse()
	dirAbs, err := filepath.Abs(*home)
	if err != nil {
		glog.Errorln("root dir wrong!", err)
		os.Exit(1)
	}

	os.Chdir(*home)
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
