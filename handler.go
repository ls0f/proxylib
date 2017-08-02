package proxylib

import (
	"io"
)

type Handler interface {
	Connect(addr string) (io.ReadWriteCloser, error)
	Clean()
}
