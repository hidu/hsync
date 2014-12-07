package hsync

import (
	"github.com/golang/glog"
	"gopkg.in/fsnotify.v1"
	"net/rpc"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type HsyncClient struct {
	client          *rpc.Client
	conf           *ClientConf
	home            string
	watcher         *fsnotify.Watcher
	events          map[string]EventType
	mu              sync.RWMutex
	conncetTryTimes int
}
type EventType int

const (
	EVENT_UPDATE = 1
	EVENT_DELETE = 2
	EVENT_CHECK  = 3
)

func NewHsyncClient(confName string) (*HsyncClient, error) {
	conf,err:=LoadClientConf(confName)
	if(err!=nil){
		return nil,err
	}
	hs := &HsyncClient{
		conf:       conf,
		home:       conf.Home,
		events:     make(map[string]EventType),
	}
	return hs, nil
}

func (hc *HsyncClient) Connect() error {
	hc.conncetTryTimes++
	glog.Infoln("connect to", hc.conf.ServerAddr, "tryTimes:", hc.conncetTryTimes)
	client, err := RpcDialHTTPPath("tcp", hc.conf.ServerAddr, rpc.DefaultRPCPath, 2*time.Second)
	if err != nil {
		glog.Warningln("connect err", err)
		return err
	}
	glog.Infoln("connect to", hc.conf.ServerAddr, "success")
	hc.conncetTryTimes = 0
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

func (hc *HsyncClient) Call(method string, args interface{}, reply interface{}) (err error) {
checkConnect:
	for hc.client == nil {
		err = hc.Connect()
		if err != nil {
			glog.Warningln("not connected,reconnecting...")
			time.Sleep(1 * time.Second)
		}
	}
	err = hc.client.Call(method, args, reply)
	glog.V(2).Infoln("Call", method, err)
	if err == rpc.ErrShutdown {
		hc.client = nil
		goto checkConnect
	}
	return err
}

func (hc *HsyncClient) RemoteSaveFile(absPath string) error {
	absName, relName, err := hc.CheckPath(absPath)
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
	if reply == 1 {
		glog.Infof("Send File [%s] suc", relName)
	} else {
		glog.Warningf("Send File [%s] failed,err:%v", relName, err)
	}
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

func (hc *HsyncClient) RemoteDel(name string) error {
	_, relPath, err := hc.CheckPath(name)
	if err != nil {
		return err
	}
	var reply int
	err = hc.Call("Trans.DeleteFile", relPath, &reply)
	if reply != 0 {
		glog.Infof("Delete [%s] suc", relPath)
	} else {
		glog.Infof("Delete [%s] failed,err=", relPath, err)
	}
	return err
}

func (hc *HsyncClient) CheckOrSend(absName string) (err error) {
	absPath, relPath, err := hc.CheckPath(absName)
	if err != nil {
		return err
	}
	if isIgnore(relPath) {
		glog.V(2).Infoln("sync ignore", relPath)
		return
	}
	remoteStat, err := hc.RemoteGetStat(absPath)
	if err != nil {
		glog.Warningln("sync getstat failed", err)
		return
	}
	var localStat FileStat
	err = fileGetStat(absPath, &localStat)
	if err != nil {
		return
	}
	if !remoteStat.Exists || localStat.Md5 != remoteStat.Md5 {
		err = hc.RemoteSaveFile(absPath)
	} else {
		glog.Infoln("Not Change", relPath)
	}
	return
}

func (hc *HsyncClient) Watch() (err error) {
	hc.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		glog.Warningln("init watcher failed", err)
		return err
	}
	defer hc.watcher.Close()

	go hc.eventLoop()

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
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		err = hc.watcher.Add(absPath)
		glog.Infoln("add watch,path=[", relPath, "]", err)
		return err
	})
}

func (hc *HsyncClient) addEvent(fileName string, eventType EventType) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.events[fileName] = eventType
}

func (hc *HsyncClient) eventLoop() {
	elist := make(map[string]EventType)
	eventHander := func() {
		glog.V(2).Info("event buffer length:", len(hc.events))
		if len(hc.events) == 0 {
			return
		}
		hc.mu.Lock()
		for k, v := range hc.events {
			elist[k] = v
			delete(hc.events, k)
		}
		hc.mu.Unlock()
		if len(elist) == 0 {
			return
		}
		for fileName, v := range elist {
			switch v {
			case EVENT_UPDATE:
				hc.RemoteSaveFile(fileName)
			case EVENT_CHECK:
				hc.CheckOrSend(fileName)
			case EVENT_DELETE:
				hc.RemoteDel(fileName)
			}
			delete(elist, fileName)
		}
	}

	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-ticker.C:
			eventHander()
		}
	}
	glog.Error("sync loop exit")
}

func (hc *HsyncClient) sync() {
	glog.Infoln("sync start")
	err := filepath.Walk(hc.home, func(path string, info os.FileInfo, err error) error {
		absPath, relPath, _ := hc.CheckPath(path)
		glog.V(2).Info("sync walk ", relPath)
		if isIgnore(relPath) {
			glog.Infoln("sync ignore", relPath, absPath)
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		hc.addEvent(absPath, EVENT_CHECK)
		return nil
	})
	glog.Infoln("sync done", err)
}

func (hc *HsyncClient) eventHander(event fsnotify.Event) {
	glog.V(2).Infoln("event", event)
	absPath, relPath, err := hc.CheckPath(event.Name)
	if err != nil || hc.conf.IsIgnore(relPath) {
		glog.V(2).Infoln("ignore ",relPath)
		return
	}
	if event.Op&fsnotify.Create == fsnotify.Create {
		hc.addEvent(absPath, EVENT_UPDATE)
	}
	if event.Op&fsnotify.Write == fsnotify.Write {
		hc.addEvent(absPath, EVENT_UPDATE)
	}
	if event.Op&fsnotify.Remove == fsnotify.Remove {
		hc.addEvent(absPath, EVENT_DELETE)
		hc.watcher.Remove(absPath)
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

func isIgnore(relName string) bool {
	if relName == "." {
		return false
	}
	if strings.HasPrefix(relName, ".") {
		return true
	}
	baseName := filepath.Base(relName)
	if strings.HasPrefix(baseName, ".") || strings.HasSuffix(baseName, "~") {
		return true
	}
	return false
}
