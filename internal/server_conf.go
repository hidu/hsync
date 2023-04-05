package internal

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/fsgo/fsconf"
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

func (cfg *ServerConf) AutoCheck() error {
	if cfg.Addr == "" {
		return errors.New("server listen addr is empty")
	}

	for _, deploy := range cfg.Deploy {
		deploy.From = strings.Trim(deploy.From, "/")
	}

	return nil
}

var _ fsconf.AutoChecker = (*ServerConf)(nil)

type ServerConfDeploy struct {
	From  string `json:"from"`
	To    string `json:"to"`
	IsDir bool
}

func LoadServerConf(name string) (cfg *ServerConf, err error) {
	fp, err := filepath.Abs(name)
	if err != nil {
		return nil, err
	}
	err = fsconf.Parse(fp, &cfg)
	if err != nil {
		return nil, err
	}
	cfg.ConfDir = filepath.Dir(fp)

	if !filepath.IsAbs(cfg.Home) {
		cfg.Home = filepath.Join(cfg.ConfDir, cfg.Home)
	}
	cfg.Home = filepath.Clean(cfg.Home)
	cfg.DeployCmd = strings.TrimSpace(strings.ReplaceAll(cfg.DeployCmd, "{pwd}", cfg.ConfDir))
	glog.V(2).Info("load cfg [", name, "]suc,", cfg)

	return cfg, nil
}

func (cfg *ServerConf) String() string {
	data, _ := json.MarshalIndent(cfg, "", "    ")
	return string(data)
}

func (cfg *ServerConf) getDeployTo(relName string) []string {
	var deployTo []string
	for _, deploy := range cfg.Deploy {
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

var ConfDemoServer = `
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
