package hsync

import (
	"net"
	"net/http"
	"net/rpc"
	"fmt"
	"github.com/golang/glog"
)

type HsyncServer struct {
	conf *ServerConf
}

func NewHsyncServer(confName string) (*HsyncServer, error) {
	conf,err:=LoadServerConf(confName)
	if(err!=nil){
		return nil,err
	}
	server := &HsyncServer{
		conf:conf,
	}
	return server, nil
}

func (server *HsyncServer) Start() {
	trans := NewTrans(server)
	rpc.Register(trans)
	rpc.HandleHTTP()
	fmt.Println("hsync server lister at ",server.conf.Addr)
	l, err := net.Listen("tcp", server.conf.Addr)
	if err != nil {
		glog.Exitln("ListenAndServe,err ", err)
	}
	http.Serve(l, nil)
}
