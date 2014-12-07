package hsync

import (
	"fmt"
	"github.com/golang/glog"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"
)


type Trans struct {
	Home     string
	events   map[string]EventType
	mu       sync.RWMutex
	server *HsyncServer
}

func NewTrans(server *HsyncServer) *Trans {
	trans := &Trans{
		server:  server,
		Home:     server.conf.Home,
		events:   make(map[string]EventType),
	}
	go trans.eventLoop()
	return trans
}

type FileStat struct {
	Mtime    time.Time
	Size     int64
	Md5      string
	FileMode os.FileMode
	Exists   bool
}

func (stat *FileStat) IsDir() bool {
	return stat.FileMode.IsDir() //&& stat.FileMode&os.ModeSymlink != 1
}

type MyFile struct {
	Name string
	Data []byte
	Stat *FileStat
	Gzip bool
}

func (f *MyFile) ToString() string {
	return fmt.Sprintf("Name:%s,Mode:%v,Size:%d", f.Name, f.Stat.FileMode, f.Stat.Size)
}

func (trans *Trans) addEvent(relName string, et EventType) {
	trans.mu.Lock()
	defer trans.mu.Unlock()
	trans.events[relName] = et
}

func (trans *Trans) cleanFileName(rel_name string) (fullName string, relName string, err error) {
	fullName, err = filepath.Abs(trans.Home + "/" + rel_name)
	if err != nil {
		return
	}
	relName, err = filepath.Rel(trans.Home, fullName)
	return
}

func (trans *Trans) FileStat(relName string, result *FileStat) (err error) {
	glog.Infoln("Call FileStat", relName)
	fullName, _, err := trans.cleanFileName(relName)
	if err != nil {
		return err
	}
	err = fileGetStat(fullName, result)
	return err
}

func (trans *Trans) CopyFile(myFile *MyFile, result *int) error {
	glog.Infoln("Call CopyFile ", myFile.ToString())
	fullName, relName, err := trans.cleanFileName(myFile.Name)
	if err != nil {
		glog.Warningln("CopyFile err:", err)
		return fmt.Errorf("wrong file name")
	}
	dir := fullName
	if !myFile.Stat.IsDir() {
		dir = filepath.Dir(fullName)
	}
	err = checkDir(dir, myFile.Stat.FileMode)
	if err != nil {
		return err
	}
	if !myFile.Stat.IsDir() {
		err = ioutil.WriteFile(fullName, myFile.Data, myFile.Stat.FileMode)
		trans.addEvent(relName, EVENT_UPDATE)
	}
	*result = 1
	return err
}

func (trans *Trans) DeleteFile(relName string, result *int) (err error) {
	glog.Infoln("Call DeleteFile", relName)
	fullName, relName, err := trans.cleanFileName(relName)
	if err != nil {
		return err
	}
	err = os.Remove(fullName)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	*result = 1
	trans.addEvent(relName, EVENT_DELETE)
	return err
}

func (trans *Trans) eventLoop() {
	elist := make(map[string]EventType)
	dealEvent := func(relName string, et EventType) {
		deployTo:=trans.server.conf.getDeployTo(relName)
		glog.V(2).Infoln("deploy",relName,"-->",deployTo)
		if(len(deployTo)>0){
			if et == EVENT_UPDATE {
				for _,to:=range deployTo{
					copyFile(to,relName)
				}
			}else if  et == EVENT_DELETE{
			
			}
		}
	}
	eventHander := func() {
		glog.V(2).Info("event buffer length:", len(trans.events))
		if len(trans.events) == 0 {
			return
		}
		trans.mu.Lock()
		for k, v := range trans.events {
			elist[k] = v
			delete(trans.events, k)
		}
		trans.mu.Unlock()
		if len(elist) == 0 {
			return
		}
		for fileName, v := range elist {
			dealEvent(fileName, v)
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
	glog.Error("trans loop exit")
}

func fileGetStat(name string, stat *FileStat) error {
	info, err := os.Stat(name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		} else {
			return err
		}
	}
	stat.Exists = true
	stat.Mtime = info.ModTime()
	stat.Size = info.Size()
	stat.FileMode = info.Mode()
	if !stat.IsDir() {
		stat.Md5 = FileMd5(name)
	}
	return nil
}

func fileGetMyFile(absPath string) (*MyFile, error) {
	stat := new(FileStat)
	err := fileGetStat(absPath, stat)
	if err != nil {
		return nil, err
	}
	f := &MyFile{
		Name: absPath,
		Stat: stat,
		Gzip: false,
	}
	if !stat.IsDir() {
		f.Data, err = ioutil.ReadFile(absPath)
		if err != nil {
			return nil, err
		}
	}
	return f, nil
}
