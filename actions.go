package myproxy

import "net/http"

type HttpsHandler interface {
	HandleConnect(req string, ctx *ProxyCtx) (*ConnectAction, string)
}

type ReqHandler interface {
	Handle(req *http.Request, ctx *ProxyCtx) (*http.Request, *http.Response)
}

type FuncReqHandler func(req *http.Request, ctx *ProxyCtx) (*http.Request, *http.Response)

func (f FuncReqHandler) Handle(req *http.Request, ctx *ProxyCtx) (*http.Request, *http.Response) {
	return f(req, ctx)
}
