package myproxy

import (
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"sync/atomic"
)

type ProxyHttpServer struct {
	Verbose            bool
	Tr                 *http.Transport
	sess               int64
	Logger             Logger
	httpsHandlers      []HttpsHandler
	CertStore          CertStorage
	ConnectDial        func(network string, addr string) (net.Conn, error)
	ConnectDialWithReq func(req *http.Request, network string, addr string) (net.Conn, error)
	reqHandlers        []ReqHandler
	respHandlers       []RespHandler
}

var hasPort = regexp.MustCompile(`:\d+$`)

func (proxy *ProxyHttpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "CONNECT" {
		proxy.handleHttps(w, r)
	} else {
		var err error
		ctx := &ProxyCtx{Req: r, Proxy: proxy, Session: atomic.AddInt64(&proxy.sess, 1)}
		ctx.Logf("Got request %v %v %v %v", r.URL.Path, r.Host, r.Method, r.URL.String())

		r, resp := proxy.filterRequest(r, ctx)
		if resp == nil {
			resp, err = ctx.RoundTrip(r)
			if err != nil {
				ctx.Error = err
			}
			if resp != nil {
				ctx.Logf("Received response %v", resp.Status)
			}
		}

		resp = proxy.filterResponse(resp, ctx)

		io.Copy(w, resp.Body)
	}

}

func (proxy *ProxyHttpServer) filterRequest(r *http.Request, ctx *ProxyCtx) (req *http.Request, resp *http.Response) {
	req = r
	for _, h := range proxy.reqHandlers {
		req, resp = h.Handle(r, ctx)
		if resp != nil {
			break
		}
	}
	return
}

func (proxy *ProxyHttpServer) filterResponse(respOrig *http.Response, ctx *ProxyCtx) (resp *http.Response) {
	resp = respOrig
	for _, h := range proxy.respHandlers {
		ctx.Resp = resp
		resp = h.Handle(resp, ctx)
	}

	return
}

func NewProxyHttpServer() *ProxyHttpServer {
	proxy := ProxyHttpServer{
		Tr:           &http.Transport{TLSClientConfig: tlsClientSkipVerify, Proxy: http.ProxyFromEnvironment},
		Logger:       log.New(os.Stderr, "", log.LstdFlags),
		reqHandlers:  []ReqHandler{},
		respHandlers: []RespHandler{},
	}

	proxy.ConnectDial = dialerFromEnv(&proxy)

	return &proxy
}
