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
	conf            *ClientConf
	watcher         *fsnotify.Watcher
	clientEvents    []*ClientEvent
	mu              sync.RWMutex
	conncetTryTimes int
	reNameEvent     *fsnotify.Event
}
type EventType int

const (
	EVENT_UPDATE = 1
	EVENT_DELETE = 2
	EVENT_RENAME = 3
	EVENT_CHECK  = 9
)

type ClientEvent struct {
	Name      string
	EventType EventType
	NameTo    string
}

func NewHsyncClient(confName string) (*HsyncClient, error) {
	conf, err := LoadClientConf(confName)
	if err != nil {
		return nil, err
	}
	hs := &HsyncClient{
		conf:         conf,
		clientEvents: make([]*ClientEvent, 0),
	}
	return hs, nil
}

func (hc *HsyncClient) NewArgs(fileName string, myFile *MyFile) *RpcArgs {
	return &RpcArgs{
		Token:    hc.conf.Token,
		FileName: fileName,
		MyFile:   myFile,
	}
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

	rv := hc.RemoteVersion()
	if rv != version {
		glog.Exitln("server version [", rv, "] != client version [", version, "]")
	}

	return nil
}
func (hc *HsyncClient) CheckPath(name string) (absPath string, relPath string, err error) {
	if !filepath.IsAbs(name) {
		absPath, err = filepath.Abs(filepath.Join(hc.conf.Home, name))
	} else {
		absPath = filepath.Clean(name)
	}
	if err != nil {
		return
	}
	relPath, err = filepath.Rel(hc.conf.Home, absPath)
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

func (hc *HsyncClient) RemoteVersion() string {
	var serverVersion string
	hc.Call("Trans.Version", version, &serverVersion)
	glog.Infoln("remote server version is", serverVersion)
	return serverVersion
}

func (hc *HsyncClient) RemoteSaveFile(absPath string) error {
	absName, relName, err := hc.CheckPath(absPath)
	if err != nil {
		return err
	}
	var index int64 = 0
sendSlice:
	f, err := fileGetMyFile(absName, index)
	if err != nil {
		glog.Warningf("Send FIle [%s] failed,get file failed,err=%v", relName, err)
		return err
	}
	f.Name = relName
	var reply int
	err = hc.Call("Trans.CopyFile", hc.NewArgs(relName, f), &reply)
	if reply == 1 {
		glog.Infof("Send File [%s] [%d/%d] suc", relName, index+1, f.Total)
		if f.Total > 1 && index+1 < f.Total {
			index++
			goto sendSlice
		}
	} else {
		glog.Warningf("Send File [%s] [%d/%d] failed,err:%v", relName, index+1, f.Total, err)
	}

	return err
}

func (hc *HsyncClient) RemoteGetStat(name string) (stat *FileStat, err error) {
	_, relName, err := hc.CheckPath(name)
	if err != nil {
		return nil, err
	}
	err = hc.Call("Trans.FileStat", hc.NewArgs(relName, nil), &stat)
	return
}

func (hc *HsyncClient) RemoteDel(name string) error {
	_, relPath, err := hc.CheckPath(name)
	if err != nil {
		return err
	}
	var reply int
	err = hc.Call("Trans.DeleteFile", hc.NewArgs(relPath, nil), &reply)
	if reply == 1 {
		glog.Infof("Delete [%s] suc", relPath)
	} else {
		glog.Infof("Delete [%s] failed,err=", relPath, err)
	}
	return err
}
func (hc *HsyncClient) RemoteReName(name string, nameOld string) error {
	_, relName, err := hc.CheckPath(name)
	if err != nil {
		return err
	}
	_, relNameOld, err := hc.CheckPath(nameOld)
	if err != nil {
		return err
	}
	f := &MyFile{Name: relNameOld}
	var reply int
	err = hc.Call("Trans.FileReName", hc.NewArgs(relName, f), &reply)
	if reply == 1 {
		glog.Infof("Rename [%s]->[%s] suc", relNameOld, relName)
	} else {
		glog.Infof("Rename [%s]->[%s] failed,err=%v", relNameOld, relName, err)
		hc.addEvent(relName, EVENT_CHECK, "")
		hc.addEvent(relNameOld, EVENT_DELETE, "")
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
				glog.Warningln("fswatch error:", err)
			}
		}
	}()
	hc.watcher.Add(hc.conf.Home)
	hc.addWatch(hc.conf.Home)

	hc.sync()

	<-done
	return nil
}

func (hc *HsyncClient) addWatch(dir string) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		absPath, relPath, _ := hc.CheckPath(path)
		//only need watch dir
		if !info.IsDir() {
			return nil
		}

		if isIgnore(relPath) || hc.conf.IsIgnore(relPath) {
			glog.Infoln("ignore watch,path=[", relPath, "]")
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

func (hc *HsyncClient) addEvent(fileName string, eventType EventType, nameTo string) {
	hc.clientEvents = append(hc.clientEvents, &ClientEvent{Name: fileName, EventType: eventType, NameTo: nameTo})
}

func (hc *HsyncClient) eventLoop() {
	eventHander := func() {
		glog.V(2).Info("event buffer length:", len(hc.clientEvents))
		if len(hc.clientEvents) == 0 {
			return
		}
		hc.mu.Lock()
		elist := make([]*ClientEvent, len(hc.clientEvents))
		copy(elist, hc.clientEvents)
		hc.clientEvents = make([]*ClientEvent, 0)
		hc.mu.Unlock()

		for _, ev := range elist {
			switch ev.EventType {
			case EVENT_UPDATE:
				hc.RemoteSaveFile(ev.Name)
			case EVENT_CHECK:
				hc.CheckOrSend(ev.Name)
			case EVENT_DELETE:
				hc.RemoteDel(ev.Name)
			case EVENT_RENAME:
				hc.RemoteReName(ev.Name, ev.NameTo)
			default:
				glog.Warningln("unknow event:", ev)
			}
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
	glog.Infoln("sync scan start")
	err := filepath.Walk(hc.conf.Home, func(path string, info os.FileInfo, err error) error {
		absPath, relPath, _ := hc.CheckPath(path)
		glog.V(2).Info("sync walk ", relPath)
		if isIgnore(relPath) {
			glog.Infoln("sync ignore", relPath)
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		hc.addEvent(absPath, EVENT_CHECK, "")
		return nil
	})
	glog.Infoln("sync scan done", err)
}

func (hc *HsyncClient) eventHander(event fsnotify.Event) {
	glog.V(2).Infoln("event", event)
	absPath, relName, err := hc.CheckPath(event.Name)
	if err != nil || hc.conf.IsIgnore(relName) {
		glog.V(2).Infoln("ignore ", relName)
		return
	}
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if event.Op&fsnotify.Create == fsnotify.Create {
		if hc.reNameEvent != nil {
			absPathOld, relNameOld, _ := hc.CheckPath(hc.reNameEvent.Name)
			hc.reNameEvent = nil
			hc.watcher.Remove(absPathOld)
			glog.V(2).Infoln("event rename", relNameOld, "->", relName)

			hc.addEvent(absPath, EVENT_RENAME, absPathOld)
		} else {
			hc.addEvent(absPath, EVENT_UPDATE, "")
		}
		stat, err := os.Stat(absPath)
		if err == nil && stat.IsDir() {
			hc.addWatch(absPath)
		}
	}

	if event.Op&fsnotify.Write == fsnotify.Write {
		hc.addEvent(absPath, EVENT_UPDATE, "")
	}

	if event.Op&fsnotify.Remove == fsnotify.Remove {
		hc.addEvent(absPath, EVENT_DELETE, "")
		hc.watcher.Remove(absPath)
	}

	if event.Op&fsnotify.Rename == fsnotify.Rename {
		hc.reNameEvent = &event
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

var _defaultIgnores = map[string]int{
	"hsync.json":  1,
	"hsyncd.json": 1,
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
	if _, has := _defaultIgnores[baseName]; has {
		return true
	}
	return false
}
