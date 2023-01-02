package myproxy

import (
	"bufio"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"sync/atomic"
)

type ProxyHttpServer struct {
	Verbose                bool
	Tr                     *http.Transport
	sess                   int64
	Logger                 Logger
	httpsHandlers          []HttpsHandler
	CertStore              CertStorage
	ConnectDial            func(network string, addr string) (net.Conn, error)
	ConnectDialWithReq     func(req *http.Request, network string, addr string) (net.Conn, error)
	reqHandlers            []ReqHandler
	respHandlers           []RespHandler
	KeepDestinationHeaders bool
	KeepHeader             bool
	NonproxyHandler        http.Handler
}

type flushWriter struct {
	w io.Writer
}

func (fw flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if f, ok := fw.w.(http.Flusher); ok {
		f.Flush()
	}
	return n, err
}

var hasPort = regexp.MustCompile(`:\d+$`)

func copyHeaders(dst, src http.Header, keepDestHeaders bool) {
	if !keepDestHeaders {
		for k := range dst {
			dst.Del(k)
		}
	}

	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

func isEof(r *bufio.Reader) bool {
	_, err := r.Peek(1)
	if err == io.EOF {
		return true
	}
	return false
}

func removeProxyHeaders(ctx *ProxyCtx, r *http.Request) {
	r.RequestURI = ""
	ctx.Logf("Sending request %v %v", r.Method, r.URL.String())

	r.Header.Del("Accept-Encoding")
	r.Header.Del("Proxy-Connection")
	r.Header.Del("Proxy-Authenticate")
	r.Header.Del("Proxy-Authorization")

	if r.Header.Get("Connection") == "close" {
		r.Close = false
	}
	r.Header.Del("Connection")
}

func (proxy *ProxyHttpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "CONNECT" {
		proxy.handleHttps(w, r)
	} else {
		var err error
		ctx := &ProxyCtx{Req: r, Proxy: proxy, Session: atomic.AddInt64(&proxy.sess, 1)}
		ctx.Logf("Got request %v %v %v %v", r.URL.Path, r.Host, r.Method, r.URL.String())
		if !r.URL.IsAbs() {
			proxy.NonproxyHandler.ServeHTTP(w, r)
			return
		}

		r, resp := proxy.filterRequest(r, ctx)
		if resp == nil {
			if isWebSocketRequest(r) {
				ctx.Logf("Request looks like websocket upgrade.")
				proxy.serveWebsocket(ctx, w, r)
			}

			if !proxy.KeepHeader {
				removeProxyHeaders(ctx, r)
			}
			resp, err = ctx.RoundTrip(r)
			if err != nil {
				ctx.Error = err
			}
			if resp != nil {
				ctx.Logf("Received response %v", resp.Status)
			}
		}

		var origBody io.ReadCloser
		if resp != nil {
			origBody = resp.Body
			defer origBody.Close()
		}

		resp = proxy.filterResponse(resp, ctx)

		if resp == nil {
			var errorString string
			if ctx.Error != nil {
				errorString = "error read response" + r.URL.Host + " : " + ctx.Error.Error()
				ctx.Logf(errorString)
				http.Error(w, ctx.Error.Error(), 500)
			} else {
				errorString = "error read response " + r.URL.Host
				ctx.Logf(errorString)
				http.Error(w, errorString, 500)
			}
			return
		}
		ctx.Logf("Copying response to client %v [%d]", resp.Status, resp.StatusCode)

		if origBody != resp.Body {
			resp.Header.Del("Content-Length")
		}

		copyHeaders(w.Header(), resp.Header, proxy.KeepDestinationHeaders)
		w.WriteHeader(resp.StatusCode)
		var copyWriter io.Writer = w
		if w.Header().Get("content-type") == "text/event-stream" {
			copyWriter = &flushWriter{w: w}
		}

		nr, err := io.Copy(copyWriter, resp.Body)
		if err := resp.Body.Close(); err != nil {
			ctx.Warnf("Can't close response body %v", err)
		}
		ctx.Logf("Copied %v bytes to client error=%v", nr, err)
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
		Tr:            &http.Transport{TLSClientConfig: tlsClientSkipVerify, Proxy: http.ProxyFromEnvironment},
		Logger:        log.New(os.Stderr, "", log.LstdFlags),
		reqHandlers:   []ReqHandler{},
		respHandlers:  []RespHandler{},
		httpsHandlers: []HttpsHandler{},
		NonproxyHandler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			http.Error(w, "This is a proxy server. Does not respond to non-proxy requests.", 500)
		}),
	}

	proxy.ConnectDial = dialerFromEnv(&proxy)

	return &proxy
}
