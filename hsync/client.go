package hsync

import (
	"github.com/golang/glog"
	"gopkg.in/fsnotify.v1"
	"net/rpc"
	"os"
	"path/filepath"
	"strings"
)

type HsyncClient struct {
	serverAddr string
	client     *rpc.Client
	home       string
	watcher    *fsnotify.Watcher
}

func NewHsyncClient(addr string, home string) (*HsyncClient, error) {
	hs := &HsyncClient{
		serverAddr: addr,
		home:       home,
	}
	return hs, nil
}

func (hc *HsyncClient) Connect() error {
	client, err := rpc.DialHTTP("tcp", hc.serverAddr)
	if err != nil {
		glog.Warningln("connect err", err)
		return err
	}
	glog.Infoln("connect ok")
	hc.client = client
	return nil
}
func (hc *HsyncClient) CheckPath(name string) (absPath string, relPath string, err error) {
	absPath, err = filepath.Abs(name)
	if err != nil {
		return
	}
	relPath, err = filepath.Rel(hc.home, absPath)
	return
}

func (hc *HsyncClient) Call(method string, args interface{}, reply interface{}) error {
	err := hc.client.Call(method, args, reply)
	glog.V(2).Infoln("Call", method, err)
	return err
}

func (hc *HsyncClient) RemoteSaveFile(name string) error {
	absName, relName, err := hc.CheckPath(name)
	if err != nil {
		return err
	}
	f, err := fileGetMyFile(absName)
	if err != nil {
		return err
	}
	f.Name = relName
	var reply int
	err = hc.Call("Trans.CopyFile", f, &reply)
	return err
}

func (hc *HsyncClient) RemoteGetStat(name string) (stat *FileStat, err error) {
	_, relName, err := hc.CheckPath(name)
	if err != nil {
		return nil, err
	}
	err = hc.Call("Trans.FileStat", relName, &stat)
	return
}

func (hc *HsyncClient) Watch() (err error) {
	hc.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		glog.Warningln("init watcher failed", err)
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
				glog.Warningln("error:", err)
			}
		}
	}()
	hc.watcher.Add(hc.home)
	hc.addWatch(hc.home)

	hc.sync()

	<-done
	return nil
}

func (hc *HsyncClient) addWatch(dir string) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		absPath, relPath, _ := hc.CheckPath(path)
		if isIgnore(relPath) || !info.IsDir() {
			return nil
		}
		err = hc.watcher.Add(absPath)
		glog.Infoln("add watch,path=[", relPath, "]", err)
		return err
	})
}

func (hc *HsyncClient) CheckOrSend(name string) (isSend bool, err error) {
	absPath, relPath, err := hc.CheckPath(name)
	if err != nil {
		return false, err
	}
	if isIgnore(relPath) {
		glog.V(2).Infoln("sync ignore", relPath)
		return
	}
	stat, err := hc.RemoteGetStat(name)
	if err != nil {
		return
	}
	var localStat FileStat
	err = fileGetStat(name, &localStat)
	if err != nil {
		return
	}
	if !stat.Exists || localStat.Md5 != stat.Md5 {
		err = hc.RemoteSaveFile(absPath)
		isSend = true
	}
	return
}

func (hc *HsyncClient) RemoteDel(name string) error {
	_, relPath, err := hc.CheckPath(name)
	if err != nil {
		return err
	}
	var reply int
	return hc.Call("Trans.DeleteFile", relPath, &reply)
}

func (hc *HsyncClient) sync() {
	glog.Infoln("sync...")
	err := filepath.Walk(hc.home, func(path string, info os.FileInfo, err error) error {
		absPath, relPath, _ := hc.CheckPath(path)
		if hc.home == absPath {
			return nil
		}
		send, err := hc.CheckOrSend(relPath)
		glog.Infoln("sync", relPath, send, "err=", err)
		return err
	})
	glog.Infoln("sync done", err)
}

func (hc *HsyncClient) eventHander(event fsnotify.Event) {
	glog.V(2).Infoln("event", event)
	absPath, relPath, err := hc.CheckPath(event.Name)
	if err != nil || isIgnore(relPath) {
		return
	}
	if event.Op&fsnotify.Create == fsnotify.Create {
		hc.handerChange(absPath)
	}
	if event.Op&fsnotify.Write == fsnotify.Write {
		hc.handerChange(absPath)
	}
	if event.Op&fsnotify.Remove == fsnotify.Remove {
		hc.watcher.Remove(absPath)
		hc.RemoteDel(absPath)
	}
}

func (hc *HsyncClient) handerChange(name string) error {
	hc.RemoteSaveFile(name)
	info, err := os.Stat(name)
	if err == nil && info.IsDir() {
		hc.addWatch(name)
	}

	return nil
}

func isIgnore(name string) bool {
	baseName := filepath.Base(name)
	if strings.HasPrefix(baseName, ".") || strings.HasSuffix(baseName, "~") {
		return true
	}
	return false
}
