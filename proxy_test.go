package proxylib

import (
	"golang.org/x/net/proxy"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"
)

const msg = "HTTP/1.1 200 OK\r\nContent-Length: 11\r\n\r\nhello,world"

type handler struct {
}

type rwc struct {
}

func (c rwc) Read(p []byte) (n int, err error) {
	copy(p, []byte(msg))
	return len(msg), io.EOF
}

func (c rwc) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (c rwc) Close() (err error) {
	return
}

func (h *handler) Connect(addr string) (conn io.ReadWriteCloser, err error) {
	return &rwc{}, err
}

func (h *handler) Clean() {

}

var (
	start bool
	lock  sync.Mutex
)

func startServer() {
	lock.Lock()
	defer lock.Unlock()
	if start {
		return
	}
	s := &Server{
		Addr: ":12580",
	}
	h := &handler{}
	s.HTTPHandler = h
	s.Socks5Handler = h
	go s.ListenAndServe()
	time.Sleep(time.Millisecond * 100)
	start = true
}

func TestServer_HTTP(t *testing.T) {

	startServer()
	proxyUrl, _ := url.Parse("http://127.0.0.1:12580")
	myClient := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyUrl)}}
	res, err := myClient.Get("http://example.com")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)
	t.Logf("body:%s", string(body))
	if string(body) != "hello,world" {
		t.Fatal("http proxy err")
	}
}

func TestServer_HTTPCONNECT(t *testing.T) {

	startServer()
	conn, err := net.Dial("tcp", "127.0.0.1:12580")
	if err != nil {
		t.Fatal(err)
	}
	conn.Write([]byte("CONNECT qq.com:443 HTTP/1.1\r\n\r\n"))
	buf := make([]byte, 1024)
	_, err = io.ReadAtLeast(conn, buf, len("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := ioutil.ReadAll(conn)
	if string(body) != msg {
		t.Fatal("http connect err")
	}
}

func TestServer_Socks5Domain(t *testing.T) {
	startServer()
	dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:12580", nil, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	httpTransport := &http.Transport{}
	httpTransport.Dial = dialer.Dial
	myClient := &http.Client{Transport: httpTransport}

	res, err := myClient.Get("http://example.com")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)
	t.Logf("body:%s", string(body))
	if string(body) != "hello,world" {
		t.Fatal("socks domain err")
	}
}

func TestServer_Socks5IPV4(t *testing.T) {
	startServer()
	dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:12580", nil, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	httpTransport := &http.Transport{}
	httpTransport.Dial = dialer.Dial
	myClient := &http.Client{Transport: httpTransport}

	res, err := myClient.Get("http://8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)
	t.Logf("body:%s", string(body))
	if string(body) != "hello,world" {
		t.Fatal("socks ipv4 err")
	}
}

func TestServer_Socks5IPV6(t *testing.T) {
	startServer()
	dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:12580", nil, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	httpTransport := &http.Transport{}
	httpTransport.Dial = dialer.Dial
	myClient := &http.Client{Transport: httpTransport}

	res, err := myClient.Get("http://[::1]:12345")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)
	t.Logf("body:%s", string(body))
	if string(body) != "hello,world" {
		t.Fatal("socks ipv6 err")
	}
}
