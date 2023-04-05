package internal

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/rpc"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/golang/glog"
)

type HSyncClient struct {
	client          *rpc.Client
	conf            *ClientConf
	watcher         *fsnotify.Watcher
	events          []*ClientEvent
	mu              sync.RWMutex
	connectTryTimes atomic.Int64
	reNameEvent     *fsnotify.Event
	fileCount       uint64
	remoteHost      *ServerHost
}

type EventType int

const (
	EventUpdate = 1
	EventDelete = 2
	EventCheck  = 3
	EventRename = 4
)

type ClientEvent struct {
	Name      string
	EventType EventType
	NameTo    string
}

func (ce *ClientEvent) AsKey() string {
	return fmt.Sprintf("%s_%d_%s", ce.Name, ce.EventType, ce.NameTo)
}

func NewHSyncClient(confName string, hostName string) (*HSyncClient, error) {
	conf, err := LoadClientConf(confName)
	if err != nil {
		return nil, err
	}
	if err = conf.Parser(); err != nil {
		return nil, err
	}
	hc := &HSyncClient{
		conf:   conf,
		events: make([]*ClientEvent, 0),
	}
	if err = hc.chooseHost(hostName); err != nil {
		return nil, err
	}
	return hc, nil
}

func (hc *HSyncClient) chooseHost(hostName string) error {
	if len(hc.conf.Hosts) == 0 {
		return errors.New("no hosts")
	}

	if hostName == "" {
		hc.remoteHost = hc.conf.Hosts["default"]
		if hc.remoteHost != nil {
			glog.Infoln("use host name: default")
			return nil
		}
		for name, h := range hc.conf.Hosts {
			glog.Infoln("use host name:", name)
			hc.remoteHost = h
			return nil
		}
	}

	hc.remoteHost = hc.conf.Hosts[hostName]
	if hc.remoteHost == nil {
		return fmt.Errorf("host=%q not found", hostName)
	}
	glog.Infoln("use host name:", hostName)
	return nil
}

func (hc *HSyncClient) NewArgs(fileName string, myFile *MyFile) *RpcArgs {
	if myFile != nil {
		myFile.Name = filepath.ToSlash(myFile.Name)
	}
	return &RpcArgs{
		Token:    hc.remoteHost.Token,
		FileName: filepath.ToSlash(fileName),
		MyFile:   myFile,
	}
}

func (hc *HSyncClient) Start() error {
	if err := hc.Connect(); err != nil {
		return err
	}
	return hc.Watch()
}

func (hc *HSyncClient) Connect() error {
	num := hc.connectTryTimes.Add(1)
	glog.Infoln("connect to", hc.remoteHost.Host, "tryTimes:", num)
	client, err := RpcDialHTTPPath("tcp", hc.remoteHost.Host, rpc.DefaultRPCPath, 2*time.Second)
	if err != nil {
		glog.Warningln("connect err", err)
		return err
	}

	glog.Infoln("connect to", hc.remoteHost.Host, "success")
	hc.connectTryTimes.Store(0)
	hc.client = client

	rv := strings.Split(hc.RemoteVersion(), " ")
	lv := strings.Split(version, " ")
	if rv[0] != lv[0] {
		glog.Exitln("server version [", rv[0], "] != client version [", lv[0], "]")
	}

	return nil
}

func (hc *HSyncClient) CheckPath(name string) (absPath string, relPath string, err error) {
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

func (hc *HSyncClient) Call(method string, args any, reply any) (err error) {
checkConnect:
	for hc.client == nil {
		err = hc.Connect()
		if err != nil {
			glog.Warningln("not connected,reconnecting...")
			time.Sleep(1 * time.Second)
		}
	}
	isTimeout := false

	timeout := time.AfterFunc(30*time.Second, func() {
		glog.Warningln("Call", method, "timeout")
		isTimeout = true
		if hc.client != nil {
			hc.client.Close()
		}
	})

	err = hc.client.Call(method, args, reply)

	glog.V(2).Infoln("Call", method, err)
	if err == rpc.ErrShutdown || isTimeout {
		hc.client = nil
		goto checkConnect
	}
	if err != nil {
		glog.Warningln("\n==============================================================")
		glog.Warningln("Call", method, "failed,", err)
		glog.Warningln("==============================================================\n")
	} else {
		timeout.Stop()
	}
	return err
}

func (hc *HSyncClient) RemoteVersion() string {
	var serverVersion string
	hc.Call("Trans.Version", version, &serverVersion)
	glog.Infoln("remote server version is", serverVersion)
	return serverVersion
}

func (hc *HSyncClient) RemoteSaveFile(absPath string) error {
	return hc.remoteSaveFile(absPath, nil)
}

func (hc *HSyncClient) RemoteFileTruncate(absPath string) error {
	absName, relName, err := hc.CheckPath(absPath)
	if err != nil {
		return err
	}
	f, err := fileGetMyFileStat(absName)
	if err != nil {
		return err
	}
	f.Name = relName
	var reply int64 = -1
	err = hc.Call("Trans.FileTruncate", hc.NewArgs(relName, f), &reply)
	return err
}

func (hc *HSyncClient) remoteSaveFile(absPath string, ignoreParts map[int64]int) error {
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

	if f.Stat.IsDir() {
		go hc.addNewDir(absName)
	}

	isNotDone := f.Total > 1 && index+1 < f.Total

	logMsg := fmt.Sprintf("Send File [%s] [%3d / %d]", relName, index+1, f.Total)

	if isNotDone && ignoreParts != nil {
		if _, has := ignoreParts[index]; has {
			glog.Infoln(logMsg, "Skip")
			index++
			goto sendSlice
		}
	}

	f.Name = relName
	var reply int
	err = hc.Call("Trans.CopyFile", hc.NewArgs(relName, f), &reply)
	if reply == 1 {
		glog.Infoln(logMsg, "Suc")
		if isNotDone {
			index++
			goto sendSlice
		}
	} else {
		glog.Warningln(logMsg, "failed,err=", err)
	}

	return err
}

func (hc *HSyncClient) RemoteGetStat(name string) (stat *FileStat, err error) {
	_, relName, err := hc.CheckPath(name)
	if err != nil {
		return nil, err
	}
	err = hc.Call("Trans.FileStat", hc.NewArgs(relName, nil), &stat)
	return
}

func (hc *HSyncClient) RemoteGetStatSlice(name string) (stat *FileStatSlice, err error) {
	_, relName, err := hc.CheckPath(name)
	if err != nil {
		return nil, err
	}
	err = hc.Call("Trans.FileStatSlice", hc.NewArgs(relName, nil), &stat)
	return
}

func (hc *HSyncClient) RemoteDel(name string) error {
	_, relPath, err := hc.CheckPath(name)
	if err != nil {
		return err
	}

	var reply int
	err = hc.Call("Trans.DeleteFile", hc.NewArgs(relPath, nil), &reply)
	if reply == 1 {
		glog.Info(relPath, " Delete suc")
	} else {
		glog.Infof("Delete [%s] failed,err=", relPath, err)
	}
	return err
}

func (hc *HSyncClient) RemoteReName(name string, nameOld string) error {
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
		hc.mu.Lock()
		defer hc.mu.Unlock()

		hc.addEvent(relName, EventCheck, "")
		hc.addEvent(relNameOld, EventDelete, "")
	}
	return err
}

func (hc *HSyncClient) CheckOrSend(absName string) (err error) {
	tk := time.NewTicker(10 * time.Second)
	defer tk.Stop()
	var done atomic.Bool
	defer func() {
		done.Store(true)
	}()
	go func() {
		for range tk.C {
			if done.Load() {
				return
			}
			log.Println("CheckOrSend not finished", absName, "")
		}
	}()

	id := atomic.AddUint64(&hc.fileCount, 1)
	absPath, relPath, err := hc.CheckPath(absName)
	if err != nil {
		return err
	}
	if isIgnore(relPath) {
		glog.V(2).Infoln("[", id, "] sync ignore", relPath)
		return
	}
remoteCheck:
	var localStat FileStat
	err = fileGetStat(absPath, &localStat, true)
	if err != nil {
		return
	}

	if !localStat.IsDir() && localStat.FileMode&os.ModeNamedPipe != 0 {
		glog.Infoln("[", id, "]", relPath, "is pipe file, ignored")
		return
	}

	remoteStat, err := hc.RemoteGetStat(absPath)
	if err != nil {
		glog.Warningln("[", id, "] sync getstat failed", err)
		return
	}

	if localStat.IsDir() && remoteStat.Exists && !remoteStat.IsDir() {
		err = hc.RemoteDel(absPath)
		glog.Infoln("[", id, "]", relPath, "local_is_dir_but_remote_is_not_dir,delete:", err)
		goto remoteCheck
	}
	if !remoteStat.Exists || localStat.Md5 != remoteStat.Md5 {
		if localStat.Size/TransMaxLength < 3 {
			err = hc.RemoteSaveFile(absPath)
		} else {
			err = hc.flashSend(absPath)
		}
	} else {
		glog.Infoln("[", id, "]", relPath, "Not Change")
	}
	return
}

func (hc *HSyncClient) flashSend(absName string) (err error) {
	absPath, relPath, err := hc.CheckPath(absName)
	if err != nil {
		return err
	}
	var localStatSlice FileStatSlice
	err = fileGetStatSlice(absPath, &localStatSlice)
	if err != nil {
		return err
	}
	remoteStatSlice, err := hc.RemoteGetStatSlice(relPath)
	if err != nil {
		return err
	}
	ignoreParts := make(map[int64]int)
	for index, statPart := range localStatSlice.Parts {
		if int64(index)+1 > remoteStatSlice.Total || statPart.Md5 != remoteStatSlice.Parts[index].Md5 {
		} else {
			ignoreParts[int64(index)] = 1
		}
	}
	err = hc.remoteSaveFile(absPath, ignoreParts)
	// 	if err == nil && localStatSlice.Size < remoteStatSlice.Size {
	// 		err = hc.RemoteFileTruncate(absPath)
	// 	}
	return err
}

func (hc *HSyncClient) Watch() (err error) {
	hc.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		glog.Warningln("init watcher failed", err)
		return err
	}
	defer hc.watcher.Close()

	go hc.eventLoop()

	go func() {
		for {
			select {
			case event := <-hc.watcher.Events:
				hc.eventHandler(event)
			case err := <-hc.watcher.Errors:
				glog.Warningln("fswatch error:", err)
			}
		}
	}()
	hc.watcher.Add(hc.conf.Home)
	hc.addWatch(hc.conf.Home)

	glog.Infoln("start sync ...")
	hc.sync()

	done := make(chan bool)
	<-done
	return nil
}

func (hc *HSyncClient) addWatch(dir string) {
	start := time.Now()
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			glog.Warningf("walk %q with error  %v and skipped", dir, err)
			return nil
		}
		absPath, relPath, _ := hc.CheckPath(path)
		// only need watch dir
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
	cost := time.Since(start)
	glog.Infoln("addWatch", dir, "finished, cost=", cost.String(), "err=", err)
}

func (hc *HSyncClient) addEvent(fileName string, eventType EventType, nameTo string) {
	hc.events = append(hc.events, &ClientEvent{Name: fileName, EventType: eventType, NameTo: nameTo})
}

var clientThreadNumber int

func init() {
	flag.IntVar(&clientThreadNumber, "tr", 20, "thread number of launchd  check")
}

func (hc *HSyncClient) eventLoop() {
	if clientThreadNumber < 1 {
		glog.Error("sync loop exit")
	}
	// 限制同时check的文件数量,以避免同时打开大量文件
	checkChan := make(chan bool, clientThreadNumber)

	eventHandler := func() {
		n := len(hc.events)
		glog.V(2).Info("event buffer length:", n)
		fmt.Print(n)
		if n == 0 {
			return
		}

		hc.mu.Lock()
		events := make([]*ClientEvent, len(hc.events))

		copy(events, hc.events)
		// @todo 需要处理一个文件，同时多种事件的情况，比如先删除再立马创建
		// 要保证处理的是有时序的
		hc.events = make([]*ClientEvent, 0)
		hc.mu.Unlock()

		eventCache := make(map[string]time.Time)

		var wg sync.WaitGroup
		for _, ev := range events {
			cacheKey := ev.AsKey()
			if t, has := eventCache[cacheKey]; has && time.Since(t).Seconds() < 5 {
				glog.V(2).Infoln("same event in loop,skip", cacheKey)
				continue
			}
			eventCache[cacheKey] = time.Now()

			switch ev.EventType {
			case EventUpdate:
				hc.RemoteSaveFile(ev.Name)
			case EventCheck:

				// hc.CheckOrSend(ev.Name)
				// 为了时序性 先这样处理
				wg.Add(1)
				checkChan <- true
				go (func(name string) {
					hc.CheckOrSend(name)
					<-checkChan
					wg.Done()
				})(ev.Name)
			case EventDelete:
				hc.RemoteDel(ev.Name)
			case EventRename:
				hc.RemoteReName(ev.Name, ev.NameTo)
			default:
				glog.Warningln("unknown event:", ev)
			}
		}
		wg.Wait()
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		eventHandler()
	}
	glog.Error("sync loop exit")
}

func (hc *HSyncClient) sync() {
	hc.addNewDir(hc.conf.Home)
}

func (hc *HSyncClient) addNewDir(dirPath string) {
	hc.addWatch(dirPath)
	glog.Infoln("sync", dirPath, "start")
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		absPath, relPath, _ := hc.CheckPath(path)
		glog.V(2).Info("sync walk ", relPath)
		if hc.conf.IsIgnore(relPath) {
			glog.Infoln("sync ignore", relPath)
			if info.IsDir() {
				hc.addWatch(absPath)
				return filepath.SkipDir
			}
			return nil
		}
		hc.mu.Lock()
		hc.addEvent(absPath, EventCheck, "")
		hc.mu.Unlock()
		return nil
	})
	glog.Infoln("sync", dirPath, "done", err)
}

func (hc *HSyncClient) eventHandler(event fsnotify.Event) {
	glog.V(2).Infoln("event", event)

	absPath, relName, err := hc.CheckPath(event.Name)
	if err != nil || hc.conf.IsIgnore(relName) {
		glog.V(2).Infoln("ignore ", relName, err)
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

			hc.addEvent(absPath, EventRename, absPathOld)
		} else {
			hc.addEvent(absPath, EventUpdate, "")
		}
		stat, err := os.Stat(absPath)
		if err == nil && stat.IsDir() {
			hc.addWatch(absPath)
		}
		// rename event emit [rename->create->write], so just return
		return
	}

	if event.Op&fsnotify.Write == fsnotify.Write {
		stat, err := os.Stat(absPath)
		if err != nil {
			glog.Warningln("get file stat failed,err=", err, "event=", event)
			return
		}
		if stat.Size() > 102400 {
			hc.addEvent(absPath, EventCheck, "")
		} else {
			hc.addEvent(absPath, EventUpdate, "")
		}
	}

	if event.Op&fsnotify.Remove == fsnotify.Remove {
		hc.addEvent(absPath, EventDelete, "")
		hc.watcher.Remove(absPath)
	}

	// now not support rename
	if event.Op&fsnotify.Rename == fsnotify.Rename {
		// 		hc.reNameEvent = &event
		hc.addEvent(absPath, EventDelete, "")
		hc.watcher.Remove(absPath)
	}

	if event.Op&fsnotify.Chmod == fsnotify.Chmod {
		hc.addEvent(absPath, EventUpdate, "")
	}
}

// func (hc *HSyncClient) handlerChange(name string) error {
// 	hc.RemoteSaveFile(name)
// 	info, err := os.Stat(name)
// 	if err == nil && info.IsDir() {
// 		hc.addWatch(name)
// 	}
//
// 	return nil
// }

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
