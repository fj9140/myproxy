package main

import (
	"bytes"
	"flag"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/fj9140/myproxy"
)

func main() {
	proxy := myproxy.NewProxyHttpServer()
	proxy.OnRequest().HandleConnect(myproxy.AlwaysMitm)
	proxy.OnResponse().DoFunc(func(resp *http.Response, ctx *myproxy.ProxyCtx) *http.Response {
		resp.Body = ioutil.NopCloser(bytes.NewBufferString("chico"))
		resp.StatusCode = http.StatusOK
		return resp
	})

	addr := flag.String("addr", ":8080", "proxy listen address")
	flag.Parse()
	log.Fatal(http.ListenAndServe(*addr, proxy))
}
