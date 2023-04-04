package internal

import (
	"bytes"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/golang/glog"
)

type HSyncServer struct {
	conf          *ServerConf
	deployCmdArgs []string
}

func NewHSyncServer(confName string) (*HSyncServer, error) {
	conf, err := LoadServerConf(confName)
	if err != nil {
		return nil, err
	}
	checkDir(conf.Home, 0755)
	err = os.Chdir(conf.Home)
	if err != nil {
		return nil, err
	}
	pwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	glog.Infoln("cwd:", pwd)
	server := &HSyncServer{
		conf: conf,
	}
	reg := regexp.MustCompile(`\s+`)
	server.deployCmdArgs = reg.Split(strings.TrimSpace(conf.DeployCmd), -1)
	return server, nil
}

func (server *HSyncServer) Start() {
	trans := NewTrans(server)
	rpc.Register(trans)
	rpc.HandleHTTP()
	glog.Infoln("hsync server listen at ", server.conf.Addr)
	l, err := net.Listen("tcp", server.conf.Addr)
	if err != nil {
		glog.Exitln("ListenAndServe,err ", err)
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		glog.Infoln("direct visit", r.RemoteAddr, r.Method, r.UserAgent(), r.Referer())
		w.Write([]byte("hsyncd is ready (v" + GetVersion() + ")"))
	})
	http.Serve(l, nil)
}

func (server *HSyncServer) DeployAll() {
	glog.Infoln("deploy all start")
	for _, dc := range server.conf.Deploy {
		server.deploy(dc.To, dc.From)
	}
	glog.Infoln("deploy all done")
}

func (server *HSyncServer) deploy(dst, src string) {
	var err error
	os.Chdir(server.conf.Home)
	err = copyFile(dst, src)
	pwd, _ := os.Getwd()
	glog.Infof("deploy Copy [%s]->[%s],err=%v, pwd=%s", src, dst, err, pwd)
	if err != nil {
		return
	}

	if server.conf.DeployCmd == "" {
		return
	}

	cmdArgs := make([]string, len(server.deployCmdArgs)-1)
	copy(cmdArgs, server.deployCmdArgs[1:])

	cmdArgs = append(cmdArgs, dst)
	cmdArgs = append(cmdArgs, src)
	cmdArgs = append(cmdArgs, "update")

	cmd := exec.Command(server.deployCmdArgs[0], cmdArgs...)
	cmd.Dir = server.conf.Home

	var out bytes.Buffer
	cmd.Stdout = &out

	var outErr bytes.Buffer
	cmd.Stderr = &outErr
	err = cmd.Run()
	glog.Infof("deployCmd [%s]->[%s],err=%v", src, dst, err)
	glog.V(2).Infoln("deployCmd", cmdArgs, "deploy stdOut:", out.String(), "stdErrOut:", outErr.String(), "err=", err)
}
