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
}

var hasPort = regexp.MustCompile(`:\d+$`)

func (proxy *ProxyHttpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "CONNECT" {
		proxy.handleHttps(w, r)
	} else {
		ctx := &ProxyCtx{Req: r, Proxy: proxy, Session: atomic.AddInt64(&proxy.sess, 1)}
		resp, err := ctx.RoundTrip(r)
		if err != nil {
			return
		}
		if resp == nil {
			return
		}
		io.Copy(w, resp.Body)
	}

}

func NewProxyHttpServer() *ProxyHttpServer {
	proxy := ProxyHttpServer{
		Tr:     &http.Transport{TLSClientConfig: tlsClientSkipVerify, Proxy: http.ProxyFromEnvironment},
		Logger: log.New(os.Stderr, "", log.LstdFlags),
	}

	proxy.ConnectDial = dialerFromEnv(&proxy)

	return &proxy
}
