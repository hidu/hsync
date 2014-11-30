package hsync

import (
	"gopkg.in/fsnotify.v1"
	"log"
	"net/rpc"
	"os"
	"path/filepath"
	"strings"
)

type HsyncClient struct {
	serverAddr string
	client     *rpc.Client
	baseDir    string
	watcher    *fsnotify.Watcher
}

func NewHsyncClient(addr string, baseDir string) (*HsyncClient, error) {

	hs := &HsyncClient{
		serverAddr: addr,
		baseDir:    baseDir,
	}
	return hs, nil
}

func (hc *HsyncClient) Connect() error {
	client, err := rpc.DialHTTP("tcp", hc.serverAddr)
	if err != nil {
		log.Println("connect err", err)
		return err
	}
	log.Println("connect ok")
	hc.client = client
	return nil
}
func (hc *HsyncClient) Call(method string, args interface{}, reply interface{}) error {
	return hc.client.Call(method, args, reply)
}

func (hc *HsyncClient) SendFile(name string) error {
	f, err := fileGetMyFile(name)
	if err != nil {
		log.Println("sendFile err:", err)
		return err
	}
	var reply int
	err = hc.Call("Trans.CopyFile", f, &reply)
	log.Println("sendFile Result", reply, err)
	return err
}

func (hc *HsyncClient) Watch() (err error) {
	hc.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		log.Println("init watcher failed", err)
		return err
	}
	defer hc.watcher.Close()
	done := make(chan bool)
	go func() {
		for {
			select {
			case event := <-hc.watcher.Events:
				hc.eventHander(event)
			case err := <-hc.watcher.Errors:
				log.Println("error:", err)
			}
		}
	}()

	hc.addWatchDir("./")
	<-done
	return nil
}

func (hc *HsyncClient) addWatchDir(dir string) {
	//	if(isIgnore(dir)){
	//		log.Println("addWatch ignore")
	//		return
	//	}
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if isIgnore(path) {
			return nil
		}
		err = hc.watcher.Add(path)
		log.Println("add watch,path=[", path, "]", err)
		return err
	})
}
func (hc *HsyncClient) eventHander(event fsnotify.Event) {
	if isIgnore(event.Name) {
		return
	}
	log.Println("event:", event)
	if event.Op&fsnotify.Write == fsnotify.Write {
		log.Println("modified file:", event.Name)
	}
	if event.Op&fsnotify.Create == fsnotify.Create {

	}
}

func isIgnore(name string) bool {
	baseName := filepath.Base(name)
	if strings.HasPrefix(baseName, ".") {
		return true
	}
	return false
}
