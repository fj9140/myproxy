package main

import (
	"bufio"
	"flag"
	"log"
	"net"
	"net/http"
	"regexp"

	"github.com/fj9140/myproxy"
)

func orPanic(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	proxy := myproxy.NewProxyHttpServer()
	proxy.OnRequest(myproxy.ReqHostMatches(regexp.MustCompile("baidu.*:443$"))).
		HandleConnect(myproxy.AlwaysReject)

	proxy.OnRequest(myproxy.ReqHostMatches(regexp.MustCompile("^.*$"))).HandleConnect(myproxy.AlwaysMitm)
	proxy.OnRequest(myproxy.ReqHostMatches(regexp.MustCompile("^.*:80$"))).HijackConnect(func(req *http.Request, client net.Conn, ctx *myproxy.ProxyCtx) {
		defer func() {
			if e := recover(); e != nil {
				ctx.Logf("error connecting to remote: %v", e)
				client.Write([]byte("HTTP/1.1 500 Cannot reach destination\r\n\r\n"))
			}
			client.Close()
		}()
		clientBuf := bufio.NewReadWriter(bufio.NewReader(client), bufio.NewWriter(client))
		remote, err := net.Dial("tcp", req.URL.Host)
		orPanic(err)
		client.Write([]byte("HTTP/1.1 200 Ok\r\n\r\n"))
		remoteBuf := bufio.NewReadWriter(bufio.NewReader(remote), bufio.NewWriter(remote))
		for {
			req, err := http.ReadRequest(clientBuf.Reader)
			orPanic(err)
			orPanic(req.Write(remoteBuf))
			orPanic(remoteBuf.Flush())
			resp, err := http.ReadResponse(remoteBuf.Reader, req)
			orPanic(err)
			orPanic(resp.Write(clientBuf.Writer))
			orPanic(clientBuf.Flush())
		}
	})

	addr := flag.String("addr", ":8080", "proxy listen address")
	flag.Parse()
	log.Fatal(http.ListenAndServe(*addr, proxy))
}
