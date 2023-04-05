package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fsgo/fsconf"
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

var _ fsconf.AutoChecker = (*ClientConf)(nil)

func (cfg *ClientConf) AutoCheck() error {
	if len(cfg.Hosts) == 0 {
		return errors.New("miss server hosts")
	}
	for name, h := range cfg.Hosts {
		h.Host = strings.TrimSpace(h.Host)
		if h.Host == "" {
			return fmt.Errorf("hosts[%s].host is empty", name)
		}
	}
	return nil
}

type ServerHost struct {
	Host  string `json:"host"`
	Token string `json:"token"`
}

func (cfg *ClientConf) String() string {
	data, _ := json.MarshalIndent(cfg, "", "    ")
	return string(data)
}

func (cfg *ClientConf) IsIgnore(relName string) bool {
	if isIgnore(relName) {
		return true
	}
	if cfg.ignoreCr.IsMatch(relName) {
		return true
	}
	if len(cfg.Allow) > 0 && !cfg.allowCr.IsMatch(relName) {
		return true
	}
	return false
}

func (cfg *ClientConf) Parser() error {
	var err error
	cfg.ignoreCr, err = NewCongRegexp(cfg.Ignore)
	if err != nil {
		return fmt.Errorf("parser Ignore: %w", err)
	}

	if len(cfg.Allow) > 0 {
		cfg.allowCr, err = NewCongRegexp(cfg.Allow)
		if err != nil {
			return fmt.Errorf("parser Allow: %w", err)
		}
	}
	return nil
}

func LoadClientConf(name string) (cfg *ClientConf, err error) {
	logErr := func(err error) {
		glog.Warningln("load cfg [", name, "]failed, err:", err)
	}

	fp, err := filepath.Abs(name)
	if err != nil {
		logErr(err)
		return nil, err
	}

	err = fsconf.Parse(fp, &cfg)
	if err != nil {
		logErr(err)
		return nil, err
	}

	cfg.ConfDir = filepath.Dir(fp)
	if !filepath.IsAbs(cfg.Home) {
		cfg.Home = filepath.Join(cfg.ConfDir, cfg.Home)
	}
	cfg.Home = filepath.Clean(cfg.Home)

	glog.V(2).Info("load cfg [", name, "] success,", cfg)
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
