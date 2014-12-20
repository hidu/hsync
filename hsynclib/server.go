package hsync

import (
	"fmt"
	"github.com/golang/glog"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"os/exec"
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
	if len(server.deployCmdArgs) == 0 {
		err = copyFile(dst, src)
	} else {
		cmdArgs := make([]string, len(server.deployCmdArgs)-1)
		copy(cmdArgs, server.deployCmdArgs[1:])
		cmdArgs = append(cmdArgs, dst)
		cmd := exec.Command(server.deployCmdArgs[0], cmdArgs...)
		err = cmd.Start()
	}
	glog.Infof("deploy [%s]->[%s],err=%v", src, dst, err)
}
