package internal

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
)

type ServerConf struct {
	Addr      string              `json:"addr"`
	Home      string              `json:"home"`
	Token     string              `json:"token"`
	Deploy    []*ServerConfDeploy `json:"deploy"`
	ConfDir   string
	DeployCmd string `json:"deployCmd"`
}

type ServerConfDeploy struct {
	From  string `json:"from"`
	To    string `json:"to"`
	IsDir bool
}

func LoadServerConf(name string) (conf *ServerConf, err error) {
	err = loadJSONFile(name, &conf)

	if err == nil {
		conf.ConfDir, err = filepath.Abs(name)
		conf.ConfDir = filepath.Dir(conf.ConfDir)
		if !filepath.IsAbs(conf.Home) {
			conf.Home = filepath.Join(conf.ConfDir, conf.Home)
		}
		conf.Home = filepath.Clean(conf.Home)
		conf.DeployCmd = strings.TrimSpace(strings.Replace(conf.DeployCmd, "{pwd}", conf.ConfDir, -1))
		conf.init()
	}
	if err == nil {
		if conf.Addr == "" {
			err = fmt.Errorf("server listen addr is empty")
		}
	}
	if err != nil {
		glog.Warningln("load conf [", name, "]failed,err:", err)
	} else {
		glog.V(2).Info("load conf [", name, "]suc,", conf)
	}

	return
}

func (conf *ServerConf) String() string {
	data, _ := json.MarshalIndent(conf, "", "    ")
	return string(data)
}

func (conf *ServerConf) init() {
	for _, deploy := range conf.Deploy {
		deploy.From = strings.Trim(deploy.From, "/")
	}
}

func (conf *ServerConf) getDeployTo(relName string) []string {
	deployTo := []string{}
	for _, deploy := range conf.Deploy {
		if deploy.From != "." && !strings.HasPrefix(relName, deploy.From) {
			continue
		}
		rel, err := filepath.Rel(deploy.From, relName)
		if err != nil {
			glog.Warningln("deploy wrong path,relName:", relName, "deploy:", deploy)
			continue
		}
		destPath := filepath.Join(deploy.To, rel)
		deployTo = append(deployTo, destPath)
	}
	return deployTo
}

var ConfDemoServer string = `
{
    "addr":":8700",
    "home":"./",
    "token":"hsynctoken201412",
    "deploy":[
        {"from":"a/","to":"d/"}
    ],
    "deployCmd":""
}
`
