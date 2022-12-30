package myproxy

import (
	"net/http"
	"strings"
)

type RespCondition interface {
	HandleResp(resp *http.Response, ctx *ProxyCtx) bool
}

type ReqCondition interface {
	RespCondition
	HandleReq(req *http.Request, ctx *ProxyCtx) bool
}

func (proxy *ProxyHttpServer) OnRequest(conds ...ReqCondition) *ReqProxyConds {
	return &ReqProxyConds{proxy, conds}
}

type ReqProxyConds struct {
	proxy    *ProxyHttpServer
	reqConds []ReqCondition
}

type ReqConditionFunc func(req *http.Request, ctx *ProxyCtx) bool

func (c ReqConditionFunc) HandleReq(req *http.Request, ctx *ProxyCtx) bool {
	return c(req, ctx)
}

func (c ReqConditionFunc) HandleResp(resp *http.Response, ctx *ProxyCtx) bool {
	return c(ctx.Req, ctx)
}

func SrcIpIs(ips ...string) ReqCondition {
	return ReqConditionFunc(func(req *http.Request, ctx *ProxyCtx) bool {
		for _, ip := range ips {
			if strings.HasPrefix(req.RemoteAddr, ip+":") {
				return true
			}
		}
		return false
	})
}

func (pcond *ReqProxyConds) Do(h ReqHandler) {
	pcond.proxy.reqHandlers = append(pcond.proxy.reqHandlers, FuncReqHandler(func(r *http.Request, ctx *ProxyCtx) (*http.Request, *http.Response) {
		for _, cond := range pcond.reqConds {
			if !cond.HandleReq(r, ctx) {
				return r, nil
			}
		}
		return h.Handle(r, ctx)
	}))
}

func (pcond *ReqProxyConds) DoFunc(f func(req *http.Request, ctx *ProxyCtx) (*http.Request, *http.Response)) {
	pcond.Do(FuncReqHandler(f))
}
