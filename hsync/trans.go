package hsync

import (
	"fmt"
	"github.com/golang/glog"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

type User struct {
	Name string
	Psw  string
	Home string
}

type Trans struct {
	Home string
}

func NewTrans(home string) *Trans {
	home, _ = filepath.Abs(home)
	return &Trans{Home: home}
}

type FileStat struct {
	Mtime    time.Time
	Size     int64
	Md5      string
	FileMode os.FileMode
	Exists   bool
}

func (stat *FileStat) IsDir() bool {
	return stat.FileMode.IsDir() && stat.FileMode&os.ModeSymlink != 1
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

func (trans *Trans) cleanFileName(name string) (fullName string, relName string, err error) {
	fullName, err = filepath.Abs(trans.Home + "/" + name)
	if err != nil {
		return
	}
	relName, err = filepath.Rel(trans.Home, fullName)
	return
}

func (trans *Trans) FileStat(name string, result *FileStat) (err error) {
	glog.Infoln("Call FileStat", name)
	fullName, _, err := trans.cleanFileName(name)
	if err != nil {
		return err
	}
	err = fileGetStat(fullName, result)
	return err
}

func (trans *Trans) CopyFile(myFile *MyFile, result *int) error {
	glog.Infoln("Call CopyFile ", myFile.ToString())
	fullName, _, err := trans.cleanFileName(myFile.Name)
	if err != nil {
		glog.Warningln("CopyFile err:", err)
		return fmt.Errorf("wrong file name")
	}
	dir := fullName
	if !myFile.Stat.IsDir() {
		dir = filepath.Dir(fullName)
	}
	_, err = os.Stat(dir)
	if os.IsNotExist(err) {
		os.MkdirAll(dir, myFile.Stat.FileMode)
	}
	if err != nil {
		return err
	}
	if !myFile.Stat.IsDir() {
		err = ioutil.WriteFile(fullName, myFile.Data, myFile.Stat.FileMode)
	}
	*result = 1
	return err
}

func (trans *Trans) DeleteFile(name string, result *int) (err error) {
	glog.Infoln("Call DeleteFile", name)
	fullName, _, err := trans.cleanFileName(name)
	if err != nil {
		return err
	}
	err = os.Remove(fullName)
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}

func fileGetStat(name string, stat *FileStat) error {
	info, err := os.Stat(name)
	if err != nil {
		return nil
	}
	stat.Exists = true
	stat.Mtime = info.ModTime()
	stat.Size = info.Size()
	stat.FileMode = info.Mode()
	if !stat.IsDir() {
		data, err := ioutil.ReadFile(name)
		if err != nil {
			return err
		}
		stat.Md5 = ByteMd5(data)
	}
	return nil
}

func fileGetMyFile(name string) (*MyFile, error) {
	stat := new(FileStat)
	err := fileGetStat(name, stat)
	if err != nil {
		return nil, err
	}
	f := &MyFile{
		Name: name,
		Stat: stat,
		Gzip: false,
	}
	if !stat.IsDir() {
		f.Data, err = ioutil.ReadFile(name)
		if err != nil {
			return nil, err
		}
	}
	return f, nil
}
