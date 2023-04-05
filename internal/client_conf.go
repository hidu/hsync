package internal

import (
	"encoding/json"
	"errors"
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
	allowCr  *ConfRegexp
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
	if len(conf.Allow) > 0 && !conf.allowCr.IsMatch(relName) {
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

func (conf *ClientConf) Parser() error {
	if len(conf.Hosts) == 0 {
		return errors.New("miss server hosts")
	}
	var err error
	conf.ignoreCr, err = NewCongRegexp(conf.Ignore)
	if err != nil {
		return fmt.Errorf("parser Ignore: %w", err)
	}

	if len(conf.Allow) > 0 {
		conf.allowCr, err = NewCongRegexp(conf.Allow)
		if err != nil {
			return fmt.Errorf("parser Allow: %w", err)
		}
	}
	return nil
}

func LoadClientConf(name string) (conf *ClientConf, err error) {
	logErr := func(err error) {
		glog.Warningln("load conf [", name, "]failed, err:", err)
	}
	err = loadJSONFile(name, &conf)
	if err != nil {
		logErr(err)
		return nil, err
	}
	fp, err := filepath.Abs(name)
	if err != nil {
		logErr(err)
		return nil, err
	}
	conf.ConfDir = filepath.Dir(fp)
	if !filepath.IsAbs(conf.Home) {
		conf.Home = filepath.Join(conf.ConfDir, conf.Home)
	}
	conf.Home = filepath.Clean(conf.Home)

	glog.V(2).Info("load conf [", name, "] success,", conf)
	return
}

var ConfDemoClient = `
{
    "hosts":{
        "default":{
           "host":"127.0.0.1:8700",
           "token":"hsyncTokenDemo@20141226"
        }
    },
    "home":"./data/",
    "allow":[],
    "ignore":[
        "a_ignore/b",
        "d_ignore/*"
    ]
}
`
