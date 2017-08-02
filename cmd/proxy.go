package main

import (
	"flag"
	"github.com/golang/glog"
	p "github.com/lovedboy/proxylib"
	"io"
	"net"
	"time"
)

type handler struct {
}

func (h *handler) Connect(addr string) (conn io.ReadWriteCloser, err error) {
	return net.DialTimeout("tcp", addr, time.Second*5)
}
func (h *handler) Clean() {}

func main() {
	addr := flag.String("addr", ":8080", "listen addr")
	flag.Parse()
	s := &p.Server{Addr: *addr}
	s.HTTPHandler = &handler{}
	s.Socks5Handler = &handler{}
	glog.V(0).Info(s.ListenAndServe())
	glog.Flush()
}
