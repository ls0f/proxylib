package proxylib

import (
	"bufio"
	"encoding/binary"
	"errors"
	"github.com/golang/glog"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
)

// Log level for glog
const (
	LFATAL = iota
	LERROR
	LWARNING
	LINFO
	LDEBUG
)

const (
	typeIPv4 = 1 // type is ipv4 address
	typeDm   = 3 // type is domain address
	typeIPv6 = 4 // type is ipv6 address
)

var (
	errNotSupportProtocol = errors.New("not support proxy protocol")
	errNotSupportNow      = errors.New("not support now")
	errAuthExtraData      = errors.New("socks authentication get extra data")
	errCmd                = errors.New("socks command not supported")
	errAddrType           = errors.New("socks addr type not supported")
	errVer                = errors.New("socks version not supported")
	errReqExtraData       = errors.New("socks request get extra data")
)

type reqReader struct {
	b []byte
	r io.Reader
}

func (r *reqReader) Read(p []byte) (n int, err error) {
	if len(r.b) == 0 {
		return r.r.Read(p)
	}
	n = copy(p, r.b)
	r.b = r.b[n:]

	return
}

type Server struct {
	Addr          string
	Socks5Handler Handler
	HTTPHandler   Handler
	DisableSocks5 bool
	DisableHTTP   bool
}

func (s *Server) handlerConn(conn net.Conn) (err error) {

	defer conn.Close()
	var (
		conn2 io.ReadWriteCloser
		n     int
	)

	buf := make([]byte, 258)
	n, err = io.ReadAtLeast(conn, buf, 2)
	if err != nil {
		return err
	}
	if buf[0] == 0x05 {
		if s.DisableSocks5 || s.Socks5Handler == nil {
			return errNotSupportProtocol
		}
		nmethod := int(buf[1])
		msgLen := nmethod + 2
		if n == msgLen {
			// common case
		} else if n < msgLen {
			if _, err = io.ReadFull(conn, buf[n:msgLen]); err != nil {
				return
			}
		} else {
			return errAuthExtraData
		}
		// send confirmation: version 5, no authentication required
		if _, err = conn.Write([]byte{0x05, 0x00}); err != nil {
			return
		}

		buf := make([]byte, 263)
		if n, err = io.ReadAtLeast(conn, buf, 5); err != nil {
			return
		}
		if buf[0] != 0x05 {
			return errVer
		}
		if buf[1] != 0x01 {
			return errCmd
		}
		reqLen := -1
		var (
			addr string
			host string
		)
		switch buf[3] {
		case typeIPv4:
			reqLen = net.IPv4len + 6
		case typeIPv6:
			reqLen = net.IPv6len + 6
		case typeDm:
			reqLen = int(buf[4]) + 7
		default:
			return errAddrType
		}
		if n == reqLen {
			// common case, do nothing
		} else if n < reqLen { // rare case
			if _, err = io.ReadFull(conn, buf[n:reqLen]); err != nil {
				return
			}
		} else {
			return errReqExtraData
		}
		switch buf[3] {
		case typeIPv4:
			host = net.IP(buf[4 : 4+net.IPv4len]).String()
		case typeIPv6:
			host = net.IP(buf[4 : 4+net.IPv6len]).String()
		case typeDm:
			host = string(buf[5 : 5+buf[4]])
		}
		port := binary.BigEndian.Uint16(buf[reqLen-2 : reqLen])
		addr = net.JoinHostPort(host, strconv.Itoa(int(port)))
		glog.V(LINFO).Infof("[socks5] %s %s", conn.RemoteAddr(), addr)
		conn2, err = s.Socks5Handler.Connect(addr)
		if err != nil {
			return
		}
		conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x08, 0x43})
		glog.V(LDEBUG).Infof("[socks5] %s <-> %s layer success", conn.RemoteAddr(), addr)
		defer s.Socks5Handler.Clean()
	} else {
		if s.DisableHTTP || s.HTTPHandler == nil {
			return errNotSupportProtocol
		}
		req, err := http.ReadRequest(bufio.NewReader(&reqReader{b: buf[:n], r: conn}))
		if err != nil {
			return err
		}
		glog.V(LINFO).Infof("[http] %s %s - %s %s", req.Method, conn.RemoteAddr(), req.Host, req.Proto)

		if glog.V(LDEBUG) {
			dump, _ := httputil.DumpRequest(req, false)
			glog.Infoln(string(dump))
		}
		if req.Method == "PRI" && req.ProtoMajor == 2 {
			conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
			return errNotSupportNow
		}
		addr := req.Host
		if !strings.Contains(addr, ":") {
			addr += ":80"
		}
		conn2, err = s.HTTPHandler.Connect(addr)
		if err != nil {
			return err
		}
		if req.Method == "CONNECT" {
			conn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
		} else {
			req.Header.Del("Proxy-Connection")
			req.Header.Set("Connection", "Keep-Alive")
			req.Write(conn2)
		}
		glog.V(LDEBUG).Infof("[http] %s <-> %s layer success", req.Method, conn.RemoteAddr(), addr)
		defer s.HTTPHandler.Clean()
	}
	defer conn2.Close()
	return s.transport(conn, conn2)
}

func (s *Server) transport(conn1 io.ReadWriter, conn2 io.ReadWriter) (err error) {
	errChan := make(chan error, 2)

	go func() {
		_, err := io.Copy(conn1, conn2)
		if err != nil {
			glog.V(LDEBUG).Info(err)
		}
		errChan <- err
	}()

	go func() {
		_, err := io.Copy(conn2, conn1)
		if err != nil {
			glog.V(LDEBUG).Info(err)
		}
		errChan <- err
	}()
	err = <-errChan
	return
}

func (s *Server) ListenAndServe() (err error) {
	l, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	for {
		if conn, err := l.Accept(); err == nil {
			go func() {
				if err := s.handlerConn(conn); err != nil {
					glog.V(LERROR).Infof("handle conn: %v", err)
				}
			}()
		} else {
			glog.V(LERROR).Infof("accept :%v", err)
		}

	}
	return nil
}
