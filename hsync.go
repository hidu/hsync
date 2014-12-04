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

var addr = flag.String("addr", "", "eg :8500,server listen addr")
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
		glog.Errorln("home dir wrong!", err)
		os.Exit(1)
	}

	err=os.Chdir(dirAbs)
	if(err!=nil){
		glog.Exitln("wrong home dir")
	}
	
	if(*addr==""){
		glog.Exitln("wrong addr")
	}
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
