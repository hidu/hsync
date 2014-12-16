package hsync

import (
	"encoding/json"
	"github.com/golang/glog"
	"io/ioutil"
	"path/filepath"
)

type ClientConf struct {
	ServerAddr string   `json:"server"`
	Home       string   `json:"home"`
	Token      string   `json:"token"`
	Allow      []string `json:"allow"`
	Ignore     []string `json:"ignore"`
	ConfDir    string
	ignoreCr   *ConfRegexp
}

func (conf *ClientConf) String() string {
	data, _ := json.MarshalIndent(conf, "", "    ")
	return string(data)
}

func (conf *ClientConf) IsIgnore(relName string) bool {
	if isIgnore(relName) {
		return true
	}
	if conf.ignoreCr.IsMatch(relName) {
		return true
	}
	return false
}

func LoadClientConf(name string) (conf *ClientConf, err error) {
	var data []byte

	data, err = ioutil.ReadFile(name)
	if err == nil {
		err = json.Unmarshal(data, &conf)
	}
	if err == nil {
		conf.ConfDir, err = filepath.Abs(name)
		conf.ConfDir = filepath.Dir(conf.ConfDir)
		if !filepath.IsAbs(conf.Home) {
			conf.Home = filepath.Join(conf.ConfDir, conf.Home)
		}
		conf.Home = filepath.Clean(conf.Home)
	}

	if conf != nil {
		conf.ignoreCr, err = NewCongRegexp(conf.Ignore)
	}

	if err != nil {
		glog.Warningln("load conf [", name, "]failed,err:", err)
	} else {
		glog.V(2).Info("load conf [", name, "]suc,", conf)
	}

	return
}

var ConfDemoClient string = `
{
    "server":"127.0.0.1:8700",
    "home":"./",
    "token":"hsynctoken201412",
    "ignore":[
        "*.exe"
    ]
}
`
