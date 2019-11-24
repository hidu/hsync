package internal

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
)

type ClientConf struct {
	Hosts    map[string]*ServerHost `json:"hosts"`
	Home     string                 `json:"home"`
	Allow    []string               `json:"allow"`
	Ignore   []string               `json:"ignore"`
	ConfDir  string
	ignoreCr *ConfRegexp
}

type ServerHost struct {
	Host  string `json:"host"`
	Token string `json:"token"`
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

func (conf *ClientConf) activeHostsString() string {
	var hosts []string
	for name, host := range conf.Hosts {
		tmp := fmt.Sprintf("%15s : %s", name, host.Host)
		hosts = append(hosts, tmp)
	}
	return strings.Join(hosts, "\n")
}

func LoadClientConf(name string) (conf *ClientConf, err error) {
	err = loadJSONFile(name, &conf)
	if err == nil {
		conf.ConfDir, err = filepath.Abs(name)
		conf.ConfDir = filepath.Dir(conf.ConfDir)
		if !filepath.IsAbs(conf.Home) {
			conf.Home = filepath.Join(conf.ConfDir, conf.Home)
		}
		conf.Home = filepath.Clean(conf.Home)

		if conf.Hosts == nil {
			err = fmt.Errorf("miss server hosts")
		}
	}

	if err == nil && conf != nil {
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
    "hosts":{
        "default":{
           "host":"127.0.0.1:8700",
           "token":"hsyncTokenDemo@20141226"
        }
    },
    "home":"./data/",
    "ignore":[
        "a_ignore/b",
        "d_ignore/*"
    ]
}
`
