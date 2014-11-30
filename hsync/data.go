package hsync

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"
)

type Trans struct {
	baseDir string
}

func NewTrans(baseDir string) *Trans {
	baseDir, _ = filepath.Abs(baseDir)
	return &Trans{baseDir: baseDir}
}

type FileStat struct {
	Mtime time.Time
	Size  int64
	Md5   string
	IsDir bool
}

type MyFile struct {
	Name string
	Data []byte
	Stat *FileStat
	Gzip bool
}

func (trans *Trans) cleanFileName(name string) (fullName string, err error) {
	absPath, err := filepath.Abs(trans.baseDir + "/" + name)
	if err != nil {
		return "", err
	}
	fullName, err = filepath.Rel(trans.baseDir, absPath)
	return
}

func (trans *Trans) FileStat(name string, result *FileStat) (err error) {
	name, err = trans.cleanFileName(name)
	if err != nil {
		return err
	}
	err = fileGetStat(name, result)
	return err
}

func (trans *Trans) CopyFile(myFile *MyFile, result *int) error {
	name, err := trans.cleanFileName(myFile.Name)
	if err != nil {
		log.Println("CopyFile err:", err)
		return fmt.Errorf("wrong file name")
	}
	err = ioutil.WriteFile(name, myFile.Data, 0644)
	*result = 1
	return err
}

func fileGetStat(name string, stat *FileStat) error {
	info, err := os.Stat(name)
	if err != nil {
		return err
	}
	stat.Mtime = info.ModTime()
	stat.Size = info.Size()
	stat.IsDir = info.IsDir()
	return nil
}

func fileGetMyFile(name string) (*MyFile, error) {
	stat := new(FileStat)
	err := fileGetStat(name, stat)
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadFile(name)
	if err != nil {
		return nil, err
	}
	f := &MyFile{
		Name: name,
		Data: data,
		Stat: stat,
		Gzip: false,
	}
	return f, nil
}
