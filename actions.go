package myproxy

type HttpsHandler interface {
	HandleConnect(req string, ctx *ProxyCtx) (*ConnectAction, string)
}
