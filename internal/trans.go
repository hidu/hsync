package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/golang/glog"
)

type transStats struct {
	success map[string]int64
	fail    map[string]int64
	last    map[string]string
	mux     sync.Mutex
}

func (ts *transStats) addWithArgs(name string, arg *RpcArgs, err error) {
	var msg string
	if arg != nil {
		msg = arg.FileName
	}
	ts.add(name, msg, err)
}

func (ts *transStats) add(name string, msg string, err error) {
	ts.mux.Lock()
	defer ts.mux.Unlock()
	if err == nil {
		ts.last[name] = "success: " + time.Now().Format(time.DateTime) + " " + msg
		ts.success[name]++
		return
	}
	ts.fail[name]++
	ts.last[name] = "fail: " + time.Now().Format(time.DateTime) + " " + msg + ", " + err.Error()
}

func (ts *transStats) String() string {
	ts.mux.Lock()
	defer ts.mux.Unlock()
	data := map[string]any{
		"Success": ts.success,
		"Fail":    ts.fail,
		"Last":    ts.last,
	}
	bf, err := json.MarshalIndent(data, " ", "  ")
	if err != nil {
		return err.Error()
	}
	return string(bf)
}

type Trans struct {
	events map[string]EventType
	mu     sync.RWMutex
	server *HSyncServer
	stats  *transStats
}

func NewTrans(server *HSyncServer) *Trans {
	trans := &Trans{
		server: server,
		events: make(map[string]EventType),
		stats: &transStats{
			success: map[string]int64{},
			fail:    map[string]int64{},
			last:    map[string]string{},
		},
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

type FileStatPart struct {
	Start int64  `json:"start"`
	Len   int64  `json:"len"`
	Md5   string `json:"md5"`
}

func (stat *FileStat) IsDir() bool {
	return stat.FileMode.IsDir() // && stat.FileMode&os.ModeSymlink != 1
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

func (trans *Trans) Stats() string {
	return trans.stats.String()
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

func (trans *Trans) checkToken(arg *RpcArgs) error {
	if trans.server.conf.Token != arg.Token {
		glog.Warningln("token not match")
		return errors.New("token not match")
	}
	arg.FileName = filepath.Clean(arg.FileName)
	if arg.MyFile != nil && arg.MyFile.Name != "" {
		arg.MyFile.Name = filepath.Clean(arg.MyFile.Name)
	}
	return nil
}

func (trans *Trans) FileStat(arg *RpcArgs, result *FileStat) (err error) {
	defer func() {
		trans.stats.addWithArgs("FileStat", arg, err)
	}()
	if err = trans.checkToken(arg); err != nil {
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
	defer func() {
		trans.stats.addWithArgs("FileReName", arg, err)
	}()
	if err = trans.checkToken(arg); err != nil {
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
		trans.addEvent(relName, EventUpdate)
		trans.addEvent(relNameOld, EventDelete)
		*result = 1
	}
	return err
}

func (trans *Trans) CopyFile(arg *RpcArgs, result *int) (err error) {
	defer func() {
		trans.stats.addWithArgs("CopyFile", arg, err)
	}()
	if err = trans.checkToken(arg); err != nil {
		return err
	}
	myFile := arg.MyFile
	// 	glog.Infoln("Call CopyFile ", myFile.ToString())
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
			trans.addEvent(relName, EventUpdate)
		}
	}
	if err != nil {
		return err
	}
	*result = 1
	return err
}

func (trans *Trans) Version(clientVersion string, v *string) (err error) {
	defer func() {
		trans.stats.add("Version", "client:"+clientVersion, err)
	}()
	glog.Infoln("trans.VersionFile,client version:", clientVersion)
	*v = version
	return nil
}

func (trans *Trans) DeleteFile(arg *RpcArgs, result *int) (err error) {
	defer func() {
		trans.stats.addWithArgs("DeleteFile", arg, err)
	}()

	if err = trans.checkToken(arg); err != nil {
		return err
	}
	if arg.FileName == "." {
		glog.Infoln("trans.DeleteFile.ignored", arg.FileName)
		return nil
	}

	fullName, relName, err := trans.cleanFileName(arg.FileName)
	glog.Infoln("trans.DeleteFile", arg.FileName, fullName)

	if err != nil {
		return err
	}
	err = os.RemoveAll(fullName)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	*result = 1
	trans.addEvent(relName, EventDelete)
	return err
}

type FileStatSlice struct {
	Size  int64           `json:"size"`
	Total int64           `json:"total"`
	Parts []*FileStatPart `json:"parts"`
}

func (trans *Trans) FileStatSlice(arg *RpcArgs, result *FileStatSlice) (err error) {
	defer func() {
		trans.stats.addWithArgs("FileStatSlice", arg, err)
	}()
	if err = trans.checkToken(arg); err != nil {
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
	defer func() {
		trans.stats.addWithArgs("FileTruncate", arg, err)
	}()
	if err = trans.checkToken(arg); err != nil {
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
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err == nil {
		*result = info.Size()
	}
	return err
}

type DirList struct {
	Files []string
}

func (trans *Trans) DirList(arg *RpcArgs, result *DirList) (err error) {
	defer func() {
		trans.stats.addWithArgs("DirList", arg, err)
	}()
	if err = trans.checkToken(arg); err != nil {
		return err
	}
	glog.Infoln("trans.FileStatSlice", arg.FileName)
	fullName, _, err := trans.cleanFileName(arg.FileName)
	if err != nil {
		return err
	}
	result = &DirList{}
	err = filepath.WalkDir(fullName, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		_, relName, err := trans.cleanFileName(path)
		if err != nil {
			return err
		}
		result.Files = append(result.Files, relName)
		if d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	return err
}

func (trans *Trans) copyEvents() map[string]EventType {
	trans.mu.Lock()
	defer trans.mu.Unlock()
	cp := maps.Clone(trans.events)
	clear(trans.events)
	return cp
}

func (trans *Trans) eventLoop() {
	dealEvent := func(relName string, et EventType) {
		deployTo := trans.server.conf.getDeployTo(relName)
		glog.Infoln("trans.eventLoop deploy", relName, "-->", deployTo)
		if len(deployTo) > 0 {
			if et == EventUpdate {
				for _, to := range deployTo {
					trans.server.deploy(to, relName)
				}
			}
			// else if et == EventDelete {
			// 	do nothing
			// }
		}
	}
	eventHandler := func() {
		events := trans.copyEvents()
		fmt.Print(len(events))
		glog.V(2).Info("trans.eventLoop event buffer length:", len(events))
		if len(events) == 0 {
			return
		}
		for fileName, v := range events {
			dealEvent(fileName, v)
		}
	}

	tm := time.NewTimer(time.Second / 2)
	defer tm.Stop()

	for range tm.C {
		eventHandler()
		tm.Reset(time.Second)
	}

	glog.Error("trans.eventLoop exit")
}

func fileGetStat(name string, stat *FileStat, md5 bool) error {
	info, err := os.Stat(name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	stat.Exists = true
	stat.Mtime = info.ModTime()
	stat.Size = info.Size()
	stat.FileMode = info.Mode()
	if stat.Size > 0 && !stat.IsDir() && md5 && stat.FileMode&os.ModeNamedPipe == 0 {
		stat.Md5 = FileMd5(name)
	}
	return nil
}

const TransMaxLength = 10485760 // 10Mb

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
		Pos:  TransMaxLength * index,
	}
	if !stat.IsDir() {
		my, err := os.Open(absPath)
		if err != nil {
			return nil, err
		}
		defer my.Close()
		f.Index = index
		f.Total = int64(math.Max(math.Ceil(float64(stat.Size)/float64(TransMaxLength)), 1)) // fix 0 bit size
		var data []byte = make([]byte, TransMaxLength)
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
		}
		return err
	}
	if info.IsDir() {
		return errors.New("not file")
	}
	my, err := os.Open(name)
	if err != nil {
		return err
	}
	defer my.Close()

	statSlice.Size = info.Size()
	statSlice.Total = int64(math.Max(math.Ceil(float64(statSlice.Size)/float64(TransMaxLength)), 1))
	statSlice.Parts = make([]*FileStatPart, 0, statSlice.Total)
	var index int64 = 0
	var pos int64 = 0

	buf := make([]byte, TransMaxLength)

	for index < statSlice.Total {
		n, err := my.ReadAt(buf, pos)
		if err != nil && err != io.EOF {
			return err
		}
		sp := new(FileStatPart)
		sp.Start = index * TransMaxLength
		sp.Len = int64(n)
		pos = sp.Start + sp.Len
		sp.Md5 = ByteMd5(buf[:n])
		statSlice.Parts = append(statSlice.Parts, sp)
		index++
	}
	return nil
}
