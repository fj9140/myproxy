package myproxy

import (
	"crypto/tls"
	"net/http"
)

type ProxyCtx struct {
	Proxy        *ProxyHttpServer
	Req          *http.Request
	Session      int64
	Error        error
	Resp         *http.Response
	RoundTripper RoundTripper
	certStore    CertStorage
}

type RoundTripper interface {
	RoundTrip(req *http.Request, ctx *ProxyCtx) (*http.Response, error)
}

type CertStorage interface {
	Fetch(hostname string, gen func() (*tls.Certificate, error)) (*tls.Certificate, error)
}

func (ctx *ProxyCtx) RoundTrip(req *http.Request) (*http.Response, error) {
	return ctx.Proxy.Tr.RoundTrip(req)
}

func (ctx *ProxyCtx) printf(msg string, argv ...interface{}) {
	ctx.Proxy.Logger.Printf("[%03d]"+msg+"\n", append([]interface{}{ctx.Session & 0xFF}, argv...)...)
}

func (ctx *ProxyCtx) Logf(msg string, argv ...interface{}) {
	if ctx.Proxy.Verbose {
		ctx.printf("INFO: "+msg, argv...)
	}
}

func (ctx *ProxyCtx) Warnf(msg string, argv ...interface{}) {
	ctx.printf("WARN: "+msg, argv...)
}
