package hsync

import (
	"log"
	"net"
	"net/http"
	"net/rpc"
)

type HsyncServer struct {
	addr    string
	baseDir string
}

func NewHsyncServer(addr string, baseDir string) (*HsyncServer, error) {
	server := &HsyncServer{
		addr:    addr,
		baseDir: baseDir,
	}
	return server, nil
}

func (server *HsyncServer) Start() {
	trans := NewTrans(server.baseDir)
	rpc.Register(trans)
	rpc.HandleHTTP()
	l, err := net.Listen("tcp", server.addr)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
	http.Serve(l, nil)
}


