package internal

import (
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/golang/glog"
)

var version string = "0.2.3 20160725"

func GetVersion() string {
	return version
}

type ConfRegexp struct {
	confs []string
	regs  []*regexp.Regexp
}

func NewCongRegexp(confs []string) (*ConfRegexp, error) {
	regs := make([]*regexp.Regexp, 0, len(confs))
	for _, cf := range confs {
		cf = strings.TrimSpace(path.Clean(cf))
		if cf == "" {
			continue
		}
		cfQuo := strings.Replace(regexp.QuoteMeta(cf), `\*`, `.*`, -1)
		if cfQuo[:1] == "/" {
			cfQuo = "^" + cfQuo[1:]
		}
		reg, err := regexp.Compile(cfQuo)

		glog.V(2).Infoln("Conf reg [", cf, "] quote as [", cfQuo, "]")

		if err != nil {
			glog.Warningln("wrong regexp:[", cf, "],skip it")
			continue
		}
		regs = append(regs, reg)
	}
	cr := &ConfRegexp{
		confs: confs,
		regs:  regs,
	}
	return cr, nil
}

func (cr *ConfRegexp) IsMatch(relName string) bool {
	relName = strings.TrimLeft(filepath.ToSlash(relName), "/")
	for _, reg := range cr.regs {
		if reg.MatchString(relName) {
			glog.V(2).Infof("match reg:[%s],relName:[%s]", reg.String(), relName)
			return true
		}
	}
	return false
}
func DemoConf(name string) string {
	if name == "server" {
		return ConfDemoServer
	}
	return ConfDemoClient

}
