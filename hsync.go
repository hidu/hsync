package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang/glog"

	hsync "github.com/hidu/hsync/internal"
)

var asDaemon = flag.Bool("d", false, "run as daemon server, default is client")
var hostName = flag.String("h", "", "host name in client config file, default is the first")
var showVersion = flag.Bool("version", false, "show version:"+hsync.GetVersion())
var demoConf = flag.String("demo_conf", "", "show default conf [client|server]")
var deployOnly = flag.Bool("deploy", false, "deploy all files for server")

func init() {
	flag.Lookup("alsologtostderr").DefValue = "true"
	flag.Set("alsologtostderr", "true")

	df := flag.Usage
	flag.Usage = func() {
		df()
		fmt.Fprintln(os.Stderr, "\n  sync dir, https://github.com/hidu/hsync/")
		fmt.Fprintln(os.Stderr, "  as client:", os.Args[0], "   [hsync.json]")
		fmt.Fprintln(os.Stderr, "  as server:", os.Args[0], "-d [hsyncd.json]")
	}
}

func main() {
	parserFlags()
	confName := getConfName()

	if *asDaemon {
		startServer(confName)
	} else {
		startClient(confName)
	}
}

func parserFlags() {
	flag.Parse()
	if *showVersion {
		fmt.Fprintln(os.Stderr, "version:", hsync.GetVersion())
		os.Exit(0)
	}

	if *demoConf != "" {
		fmt.Println(hsync.DemoConf(*demoConf))
		os.Exit(0)
	}
	if *deployOnly {
		*asDaemon = true
	}
}

func startServer(confName string) {
	server, err := hsync.NewHSyncServer(confName)
	if err != nil {
		glog.Exitln("start server failed:", err)
	}

	if *deployOnly {
		server.DeployAll()
		return
	}
	glog.Exitln("server exit:", server.Start())
}

func startClient(confName string) {
	client, err := hsync.NewHSyncClient(confName, *hostName)
	if err != nil {
		glog.Exitln("start hsync client failed:", err)
	}
	glog.Exitln("client exit:", client.Start())
}

func getConfName() string {
	confName := flag.Arg(0)
	if confName == "" {
		if *asDaemon {
			confName = "hsyncd.json"
		} else {
			confName = "hsync.json"
		}
	}

	name, err := filepath.Abs(confName)
	if err != nil {
		glog.Exitf("hsync conf [%s] not exists!", confName)
	}
	return name
}
