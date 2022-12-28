package myproxy

import "net/http"

type ProxyHttpServer struct {
	Verbose bool
}

func (proxy *ProxyHttpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {}

func NewProxyHttpServer() *ProxyHttpServer {
	proxy := ProxyHttpServer{}

	return &proxy
}
