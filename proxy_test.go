package proxylib

import (
	"golang.org/x/net/proxy"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
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

func TestServer_ListenAndServe(t *testing.T) {

	s := &Server{
		Addr: ":12580",
	}

	h := &handler{}
	s.HTTPHandler = h
	go s.ListenAndServe()
	time.Sleep(time.Millisecond * 100)
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

	s.Socks5Handler = h
	dialer, err := proxy.SOCKS5("tcp", ":12580", nil, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	httpTransport := &http.Transport{}
	httpTransport.Dial = dialer.Dial
	myClient = &http.Client{Transport: httpTransport}

	res, err = myClient.Get("http://example.com")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ = ioutil.ReadAll(res.Body)
	t.Logf("body:%s", string(body))
	if string(body) != "hello,world" {
		t.Fatal("socks proxy err")
	}
}
