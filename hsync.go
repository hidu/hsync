/**
* sync dir
* author: hidu <duv123+git@gmail.com>
* https://github.com/hidu/hsync
 */

package main

import (
	"flag"
	"github.com/hidu/hsync/hsynclib"
	//	"./hsync"
	"fmt"
	"github.com/golang/glog"
	"os"
)

var d = flag.Bool("d", false, "run model,defaul is client")
var ve = flag.Bool("version", false, "show version:"+hsync.GetVersion())
var demoConf = flag.String("demo_conf", "", "show default conf [client|server]")

func init() {
	flag.Set("logtostderr", "1")
	df := flag.Usage
	flag.Usage = func() {
		df()
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "\n  sync dir, https://github.com/hidu/hsync/")
		fmt.Fprintln(os.Stderr, "  as client:", os.Args[0], "   [hsync.json]")
		fmt.Fprintln(os.Stderr, "  as server:", os.Args[0], "-d [hsyncd.json]")
	}
}

func main() {
	flag.Parse()

	if *ve {
		fmt.Fprintln(os.Stderr, "version:", hsync.GetVersion())
		os.Exit(0)
	}
	if *demoConf != "" {
		fmt.Println(hsync.DemoConf(*demoConf))
		os.Exit(0)
	}

	confName := flag.Arg(0)
	if confName == "" {
		if *d {
			confName = "hsyncd.json"
		} else {
			confName = "hsync.json"
		}
	}

	confInfo, err := os.Stat(confName)
	if err != nil || confInfo.IsDir() {
		glog.Exitln("hsync conf not")
	}

	if *d {
		server, err := hsync.NewHsyncServer(confName)
		if err != nil {
			glog.Exitln("start server failed:", err)
		}
		server.Start()
	} else {
		client, err := hsync.NewHsyncClient(confName)
		if err != nil {
			glog.Exitln("start hsync client failed:", err)
		}
		client.Connect()
		client.Watch()
	}
}
