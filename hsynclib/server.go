package hsync

import (
	"bytes"
	"fmt"
	"github.com/golang/glog"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

type HsyncServer struct {
	conf          *ServerConf
	deployCmdArgs []string
}

func NewHsyncServer(confName string) (*HsyncServer, error) {
	conf, err := LoadServerConf(confName)
	if err != nil {
		return nil, err
	}
	err = os.Chdir(conf.Home)
	if err != nil {
		return nil, err
	}
	server := &HsyncServer{
		conf: conf,
	}
	reg := regexp.MustCompile(`\s+`)
	server.deployCmdArgs = reg.Split(strings.TrimSpace(conf.DeployCmd), -1)
	return server, nil
}

func (server *HsyncServer) Start() {
	trans := NewTrans(server)
	rpc.Register(trans)
	rpc.HandleHTTP()
	fmt.Println("hsync server listen at ", server.conf.Addr)
	l, err := net.Listen("tcp", server.conf.Addr)
	if err != nil {
		glog.Exitln("ListenAndServe,err ", err)
	}
	http.Serve(l, nil)
}

func (server *HsyncServer) deploy(dst, src string) {
	var err error
	err = copyFile(dst, src)
	if err == nil {
		cmdArgs := make([]string, len(server.deployCmdArgs)-1)
		copy(cmdArgs, server.deployCmdArgs[1:])

		cmdArgs = append(cmdArgs, dst)

		cmdArgs = append(cmdArgs, src)
		cmd := exec.Command(server.deployCmdArgs[0], cmdArgs...)

		var out bytes.Buffer
		cmd.Stdout = &out

		var outErr bytes.Buffer
		cmd.Stderr = &outErr

		err = cmd.Run()
		glog.V(2).Infoln("deploy stdOut:", out.String(), "stdErrOut:", outErr.String(), "err=", err)
	}
	glog.Infof("deploy [%s]->[%s],err=%v", src, dst, err)
}
