package internal

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/golang/glog"
)

type Trans struct {
	events map[string]EventType
	mu     sync.RWMutex
	server *HsyncServer
}

func NewTrans(server *HsyncServer) *Trans {
	trans := &Trans{
		server: server,
		events: make(map[string]EventType),
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

type RpcArgs struct {
	Token    string
	FileName string
	MyFile   *MyFile
}

type FileStatSlice struct {
	Size  int64           `json:"size"`
	Total int64           `json:"total"`
	Parts []*FileStatPart `json:"parts"`
}

type FileStatPart struct {
	Start int64  `json:"start"`
	Len   int64  `json:"len"`
	Md5   string `json:"md5"`
}

func (stat *FileStat) IsDir() bool {
	return stat.FileMode.IsDir() //&& stat.FileMode&os.ModeSymlink != 1
}

type MyFile struct {
	Name  string
	Data  []byte
	Stat  *FileStat
	Gzip  bool
	Total int64
	Index int64
	Pos   int64
}

func (f *MyFile) ToString() string {
	return fmt.Sprintf("Name:%s,Mode:%v,Size:%d", f.Name, f.Stat.FileMode, f.Stat.Size)
}

func (trans *Trans) addEvent(relName string, et EventType) {
	trans.mu.Lock()
	defer trans.mu.Unlock()
	trans.events[relName] = et
}

func (trans *Trans) cleanFileName(fileName string) (absPath string, relName string, err error) {
	if filepath.IsAbs(fileName) {
		absPath = filepath.Clean(fileName)
	} else {
		absPath, err = filepath.Abs(filepath.Join(trans.server.conf.Home, fileName))
	}
	if err != nil {
		return
	}
	relName, err = filepath.Rel(trans.server.conf.Home, absPath)
	return
}
func (trans *Trans) checkToken(arg *RpcArgs) (bool, error) {
	if trans.server.conf.Token != arg.Token {
		glog.Warningln("token not match")
		return false, fmt.Errorf("token not match")
	}
	arg.FileName = filepath.Clean(arg.FileName)
	if arg.MyFile != nil && arg.MyFile.Name != "" {
		arg.MyFile.Name = filepath.Clean(arg.MyFile.Name)
	}
	return true, nil
}

func (trans *Trans) FileStat(arg *RpcArgs, result *FileStat) (err error) {
	if suc, err := trans.checkToken(arg); !suc {
		return err
	}
	glog.Infoln("trans.FileStat", arg.FileName)
	fullName, _, err := trans.cleanFileName(arg.FileName)
	if err != nil {
		return err
	}
	err = fileGetStat(fullName, result, true)
	return err
}
func (trans *Trans) FileReName(arg *RpcArgs, result *int) (err error) {
	if suc, err := trans.checkToken(arg); !suc {
		return err
	}
	glog.Infoln("trans.FileReName", arg.MyFile.Name, "->", arg.FileName)
	fullName, relName, err := trans.cleanFileName(arg.FileName)
	if err != nil {
		return err
	}
	fullNameOld, relNameOld, err := trans.cleanFileName(arg.MyFile.Name)
	if err != nil {
		return err
	}
	err = os.Rename(fullNameOld, fullName)
	if err == nil {
		trans.addEvent(relName, EVENT_UPDATE)
		trans.addEvent(relNameOld, EVENT_DELETE)
		*result = 1
	}
	return err
}

func (trans *Trans) CopyFile(arg *RpcArgs, result *int) error {
	if suc, err := trans.checkToken(arg); !suc {
		return err
	}
	myFile := arg.MyFile
	//	glog.Infoln("Call CopyFile ", myFile.ToString())
	fullName, relName, err := trans.cleanFileName(arg.FileName)

	defer func() {
		if err == nil {
			glog.Infof("trans.CopyFile receiver file [%s] [%d/%d] suc,size:%d", relName, myFile.Index+1, myFile.Total, len(myFile.Data))
		} else {
			glog.Warningf("trans.CopyFile receiver file [%s] [%d/%d] failed,err:%v", relName, myFile.Index+1, myFile.Total, err)
		}
	}()

	if err != nil {
		return fmt.Errorf("trans.CopyFile wrong file name,err:%s", err.Error())
	}
	if myFile.Stat.IsDir() {
		err = checkDir(fullName, myFile.Stat.FileMode)
	} else {
		err = checkDir(filepath.Dir(fullName), 0755)
		var data []byte
		if myFile.Gzip {
			data = dataGzipDecode(myFile.Data)
		} else {
			data = myFile.Data
		}
		info, err := os.Stat(fullName)
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			}
		}
		if !myFile.Stat.IsDir() && info != nil && info.IsDir() {
			err = os.RemoveAll(fullName)
			glog.Infoln("trans.CopyFile | removeAll (%s) exists and not dir,because source is dir,err=", err)
			if err != nil {
				return err
			}
		}
		var f *os.File
		f, err = os.OpenFile(fullName, os.O_RDWR|os.O_CREATE, myFile.Stat.FileMode)
		if err != nil {
			return err
		}
		defer f.Close()
		n, err := f.WriteAt(data, myFile.Pos)
		if err != nil {
			return err
		}
		if n != len(data) {
			return fmt.Errorf("trans.CopyFile part of the data wrote failed,expect len=%d,now len=%d", len(data), n)
		}
		if myFile.Total == 0 || myFile.Index+1 == myFile.Total {
			err = f.Truncate(myFile.Stat.Size)
			if err != nil {
				return err
			}
			trans.addEvent(relName, EVENT_UPDATE)
		}
	}
	if err != nil {
		return err
	}
	*result = 1
	return err
}

func (trans *Trans) Version(clientVersion string, v *string) error {
	glog.Infoln("trans.Version,client version:", clientVersion)
	*v = version
	return nil
}

func (trans *Trans) DeleteFile(arg *RpcArgs, result *int) (err error) {
	if suc, err := trans.checkToken(arg); !suc {
		return err
	}
	glog.Infoln("trans.DeleteFile", arg.FileName)
	fullName, relName, err := trans.cleanFileName(arg.FileName)
	if err != nil {
		return err
	}
	err = os.RemoveAll(fullName)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	*result = 1
	trans.addEvent(relName, EVENT_DELETE)
	return err
}

func (trans *Trans) FileStatSlice(arg *RpcArgs, result *FileStatSlice) (err error) {
	if suc, err := trans.checkToken(arg); !suc {
		return err
	}
	glog.Infoln("trans.FileStatSlice", arg.FileName)
	fullName, _, err := trans.cleanFileName(arg.FileName)
	if err != nil {
		return err
	}
	err = fileGetStatSlice(fullName, result)
	return err
}

func (trans *Trans) FileTruncate(arg *RpcArgs, result *int64) (err error) {
	if suc, err := trans.checkToken(arg); !suc {
		return err
	}
	glog.Infoln("trans.FileStatSlice", arg.FileName)
	fullName, _, err := trans.cleanFileName(arg.FileName)
	if err != nil {
		return err
	}
	f, err := os.Open(fullName)
	if err != nil {
		return err
	}

	defer f.Close()
	err = f.Truncate(arg.MyFile.Stat.Size)
	if err == nil {
		info, err := f.Stat()
		if err == nil {
			*result = info.Size()
		}
	}
	return err
}

func (trans *Trans) eventLoop() {
	eventList := make(map[string]EventType)
	dealEvent := func(relName string, et EventType) {
		deployTo := trans.server.conf.getDeployTo(relName)
		glog.V(2).Infoln("trans.eventLoop deploy", relName, "-->", deployTo)
		if len(deployTo) > 0 {
			if et == EVENT_UPDATE {
				for _, to := range deployTo {
					trans.server.deploy(to, relName)
				}
			} else if et == EVENT_DELETE {

			}
		}
	}
	eventHandler := func() {
		glog.V(2).Info("trans.eventLoop event buffer length:", len(trans.events))
		if len(trans.events) == 0 {
			return
		}
		trans.mu.Lock()
		for k, v := range trans.events {
			eventList[k] = v
			delete(trans.events, k)
		}
		trans.mu.Unlock()
		if len(eventList) == 0 {
			return
		}
		for fileName, v := range eventList {
			dealEvent(fileName, v)
			delete(eventList, fileName)
		}
	}

	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-ticker.C:
			eventHandler()
		}
	}
	glog.Error("trans.eventLoop exit")
}

func fileGetStat(name string, stat *FileStat, md5 bool) error {
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
	if !stat.IsDir() && md5 {
		stat.Md5 = FileMd5(name)
	}
	return nil
}

const TRANS_MAX_LENGTH = 10485760 //10Mb

func fileGetMyFile(absPath string, index int64) (*MyFile, error) {
	stat := new(FileStat)
	md5 := false
	if index == 0 {
		md5 = true
	}

	err := fileGetStat(absPath, stat, md5)
	if err != nil {
		return nil, err
	}
	f := &MyFile{
		Name: absPath,
		Stat: stat,
		Gzip: false,
		Pos:  TRANS_MAX_LENGTH * index,
	}
	if !stat.IsDir() {
		my, err := os.Open(absPath)
		if err != nil {
			return nil, err
		}
		defer my.Close()
		f.Index = index
		f.Total = int64(math.Max(math.Ceil(float64(stat.Size)/float64(TRANS_MAX_LENGTH)), 1)) //fix 0 bit size
		var data []byte = make([]byte, TRANS_MAX_LENGTH)
		n, err := my.ReadAt(data, f.Pos)
		if err != nil && err != io.EOF {
			return nil, err
		}
		f.Data = dataGzipEncode(data[:n])
		f.Gzip = true
	}
	return f, nil
}

func fileGetMyFileStat(absPath string) (*MyFile, error) {
	stat := new(FileStat)
	err := fileGetStat(absPath, stat, false)
	if err != nil {
		return nil, err
	}
	f := &MyFile{
		Name: absPath,
		Stat: stat,
	}
	return f, nil
}

func fileGetStatSlice(name string, statSlice *FileStatSlice) error {
	info, err := os.Stat(name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		} else {
			return err
		}
	}
	if info.IsDir() {
		return fmt.Errorf("not file")
	}
	my, err := os.Open(name)
	if err != nil {
		return err
	}
	defer my.Close()

	statSlice.Size = info.Size()
	statSlice.Total = int64(math.Max(math.Ceil(float64(statSlice.Size)/float64(TRANS_MAX_LENGTH)), 1))
	statSlice.Parts = make([]*FileStatPart, 0, statSlice.Total)
	var index int64 = 0
	var pos int64 = 0

	buf := make([]byte, TRANS_MAX_LENGTH)

	for index < statSlice.Total {
		n, err := my.ReadAt(buf, pos)
		if err != nil && err != io.EOF {
			return err
		}
		sp := new(FileStatPart)
		sp.Start = index * TRANS_MAX_LENGTH
		sp.Len = int64(n)
		pos = sp.Start + sp.Len
		sp.Md5 = ByteMd5(buf[:n])
		statSlice.Parts = append(statSlice.Parts, sp)
		index++
	}
	return nil
}
