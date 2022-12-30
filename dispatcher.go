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

type ProxyConds struct {
	proxy     *ProxyHttpServer
	reqConds  []ReqCondition
	respConds []RespCondition
}

func (proxy *ProxyHttpServer) OnRequest(conds ...ReqCondition) *ReqProxyConds {
	return &ReqProxyConds{proxy, conds}
}
func (proxy *ProxyHttpServer) OnResponse(conds ...RespCondition) *ProxyConds {
	return &ProxyConds{proxy, make([]ReqCondition, 0), conds}
}

type ReqProxyConds struct {
	proxy    *ProxyHttpServer
	reqConds []ReqCondition
}

type ReqConditionFunc func(req *http.Request, ctx *ProxyCtx) bool
type RespConditionFunc func(resp *http.Response, ctx *ProxyCtx) bool

func (c ReqConditionFunc) HandleReq(req *http.Request, ctx *ProxyCtx) bool {
	return c(req, ctx)
}

func (c ReqConditionFunc) HandleResp(resp *http.Response, ctx *ProxyCtx) bool {
	return c(ctx.Req, ctx)
}

func (c RespConditionFunc) HandleResp(resp *http.Response, ctx *ProxyCtx) bool {
	return c(resp, ctx)
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

func UrlIs(urls ...string) ReqConditionFunc {
	urlSet := make(map[string]bool)
	for _, u := range urls {
		urlSet[u] = true
	}
	return func(req *http.Request, ctx *ProxyCtx) bool {
		_, pathOk := urlSet[req.URL.Path]
		_, hostAndOk := urlSet[req.URL.Host+req.URL.Path]
		return pathOk || hostAndOk
	}
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

func (pcond *ProxyConds) Do(h RespHandler) {
	pcond.proxy.respHandlers = append(pcond.proxy.respHandlers, FuncRespHandler(func(resp *http.Response, ctx *ProxyCtx) *http.Response {
		for _, cond := range pcond.reqConds {
			if !cond.HandleReq(ctx.Req, ctx) {
				return resp
			}
		}
		for _, cond := range pcond.respConds {
			if !cond.HandleResp(resp, ctx) {
				return resp
			}
		}
		return h.Handle(resp, ctx)
	}))
}

func (pcond *ProxyConds) DoFunc(f func(resp *http.Response, ctx *ProxyCtx) *http.Response) {
	pcond.Do(FuncRespHandler(f))
}
