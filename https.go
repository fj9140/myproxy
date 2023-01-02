package myproxy

import (
	"bufio"
	"crypto/tls"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type ConnectActionLiteral int

type ConnectAction struct {
	Action    ConnectActionLiteral
	Hijack    func(req *http.Request, client net.Conn, ctx *ProxyCtx)
	TLSConfig func(host string, ctx *ProxyCtx) (*tls.Config, error)
}

const (
	ConnectAccept = iota
	ConnectReject
	ConnectMitm
)

var (
	OkConnect   = &ConnectAction{Action: ConnectAccept, TLSConfig: TLSConfigFromCA(&MyproxyCa)}
	MitmConnect = &ConnectAction{Action: ConnectMitm, TLSConfig: TLSConfigFromCA(&MyproxyCa)}
	httpsRegexp = regexp.MustCompile(`^https:\/\/`)
)

type halfClosable interface {
	net.Conn
	CloseWrite() error
	CloseRead() error
}

func httpError(w io.WriteCloser, ctx *ProxyCtx, err error) {
	if _, err := io.WriteString(w, "HTTP/1.1 502 Bad Gateway\r\n\r\n"); err != nil {
		ctx.Warnf("Error responding to client: %s", err)
	}
	if err := w.Close(); err != nil {
		ctx.Warnf("Error closing client connection: %s", err)
	}
}

func (proxy *ProxyHttpServer) handleHttps(w http.ResponseWriter, r *http.Request) {
	ctx := &ProxyCtx{Req: r, Session: atomic.AddInt64(&proxy.sess, 1), Proxy: proxy, certStore: proxy.CertStore}

	hij, ok := w.(http.Hijacker)
	if !ok {
		panic("httpserver does not support hijacking")
	}

	proxyClient, _, e := hij.Hijack()
	if e != nil {
		panic("Cannot hijack connection " + e.Error())
	}

	ctx.Logf("Running %d CONNECT handlers", len(proxy.httpsHandlers))
	todo, host := OkConnect, r.URL.Host
	for i, h := range proxy.httpsHandlers {
		newtodo, newhost := h.HandleConnect(host, ctx)
		if newtodo != nil {
			todo, host = newtodo, newhost
			ctx.Logf("on %dth handler: %v %s", i, todo, host)
			break
		}
	}
	switch todo.Action {
	case ConnectAccept:
		if !hasPort.MatchString(host) {
			host += ":80"
		}
		targetSiteCon, err := proxy.connectDial(ctx, "tcp", host)
		if err != nil {
			httpError(proxyClient, ctx, err)
			return
		}
		ctx.Logf("Accepting CONNECT to %s", host)
		proxyClient.Write([]byte("HTTP/1.0 200 Connection established\r\n\r\n"))

		targetTCP, targetOK := targetSiteCon.(halfClosable)
		proxyClientTCP, clientOK := proxyClient.(halfClosable)
		if targetOK && clientOK {
			go copyAndClose(ctx, targetTCP, proxyClientTCP)
			go copyAndClose(ctx, proxyClientTCP, targetTCP)
		} else {
			go func() {
				var wg sync.WaitGroup
				wg.Add(2)
				go copyOrWarn(ctx, targetSiteCon, proxyClient, &wg)
				go copyOrWarn(ctx, proxyClient, targetSiteCon, &wg)
				wg.Wait()
				proxyClient.Close()
				targetSiteCon.Close()
			}()
		}
	case ConnectMitm:
		proxyClient.Write([]byte("HTTP/1.0 200 OK\r\n\r\n"))
		ctx.Logf("Assuming CONNECT is TLS, mitm proxying it")
		tlsConfig := defaultTLSConfig
		if todo.TLSConfig != nil {
			var err error
			tlsConfig, err = todo.TLSConfig(host, ctx)
			if err != nil {
				httpError(proxyClient, ctx, err)
				return
			}
		}
		go func() {
			rawClientTls := tls.Server(proxyClient, tlsConfig)
			if err := rawClientTls.Handshake(); err != nil {
				ctx.Warnf("Cannot handshake client %v %v", r.Host, err)
				return
			}
			defer rawClientTls.Close()
			clientTlsReader := bufio.NewReader(rawClientTls)
			for !isEof(clientTlsReader) {
				req, err := http.ReadRequest(clientTlsReader)
				var ctx = &ProxyCtx{Req: req, Session: atomic.AddInt64(&proxy.sess, 1), Proxy: proxy, UserData: ctx.UserData}
				if err != nil && err != io.EOF {
					return
				}
				if err != nil {
					ctx.Warnf("Cannot read TLS request from mitm'd client %v %v", r.Host, err)
					return
				}
				req.RemoteAddr = r.RemoteAddr
				ctx.Logf("req %v", r.Host)

				if !httpsRegexp.MatchString(req.URL.String()) {
					req.URL, err = url.Parse("https://" + r.Host + req.URL.String())
				}

				ctx.Req = req

				req, resp := proxy.filterRequest(req, ctx)
				if resp == nil {
					if err != nil {
						ctx.Warnf("Illegal URL %s", "https://"+r.Host+req.URL.Path)
						return
					}
					removeProxyHeaders(ctx, req)
					resp, err = ctx.RoundTrip(req)
					if err != nil {
						ctx.Warnf("Cannot read TLS response from mitm'd server %v", err)
						return
					}
					ctx.Logf("resp %v", resp.Status)
				}
				resp = proxy.filterResponse(resp, ctx)
				defer resp.Body.Close()

				text := resp.Status
				statusCode := strconv.Itoa(resp.StatusCode) + " "
				if strings.HasPrefix(text, statusCode) {
					text = text[len(statusCode):]
				}

				if _, err := io.WriteString(rawClientTls, "HTTP/1.1"+" "+statusCode+text+"\r\n"); err != nil {
					ctx.Warnf("Cannot write TLS response HTTP status from mitm'd client: %v", err)
					return
				}

				if resp.Request.Method == "HEAD" {

				} else {
					resp.Header.Del("Content-Length")
					resp.Header.Set("Transfer-Encoding", "chunked")
				}
				resp.Header.Set("Connection", "close")
				if err := resp.Header.Write(rawClientTls); err != nil {
					ctx.Warnf("Cannot write TLS response header from mitm'd client: %v", err)
					return
				}
				if _, err = io.WriteString(rawClientTls, "\r\n"); err != nil {
					ctx.Warnf("Cannot write TLS response header end from mitm'd client: %v", err)
					return
				}

				if resp.Request.Method == "HEAD" {

				} else {
					chunked := newChunkedWriter(rawClientTls)
					if _, err = io.Copy(chunked, resp.Body); err != nil {
						ctx.Warnf("Cannot write TLS response body from mitm'd client: %v", err)
						return
					}
					if err := chunked.Close(); err != nil {
						ctx.Warnf("Cannot write TLS chunked EOF from mitm'd client: %v", err)
						return
					}
					if _, err = io.WriteString(rawClientTls, "\r\n"); err != nil {
						ctx.Warnf("Cannot write TLS response chunked trailer form mitm'd client: %v", err)
						return
					}
				}
			}
			ctx.Logf("Exiting on EOF")
		}()
	}

}

func copyAndClose(ctx *ProxyCtx, dst, src halfClosable) {
	if _, err := io.Copy(dst, src); err != nil {
		ctx.Warnf("Error copy to client: %s", err)
	}
	dst.CloseWrite()
	src.CloseRead()
}

func copyOrWarn(ctx *ProxyCtx, dst io.Writer, src io.Reader, wg *sync.WaitGroup) {
	if _, err := io.Copy(dst, src); err != nil {
		ctx.Warnf("Error copying to client: %s", err)
	}
	wg.Done()
}

func stripPort(s string) string {
	var ix int
	if strings.Contains(s, "[") && strings.Contains(s, "]") {
		s = strings.ReplaceAll(s, "[", "")
		s = strings.ReplaceAll(s, "]", "")

		ix = strings.LastIndex(s, ":")
		if ix == -1 {
			return s
		}
	} else {
		ix = strings.IndexRune(s, ':')
		if ix == -1 {
			return s
		}
	}
	return s[:ix]
}

func dialerFromEnv(proxy *ProxyHttpServer) func(network, addr string) (net.Conn, error) {
	https_proxy := os.Getenv("HTTPS_PROXY")
	if https_proxy == "" {
		https_proxy = os.Getenv("https_proxy")
	}
	if https_proxy == "" {
		return nil
	}
	return proxy.NewConnectDialToProxy(https_proxy)
}

func (proxy *ProxyHttpServer) NewConnectDialToProxy(https_proxy string) func(network, addr string) (net.Conn, error) {
	return proxy.NewConnectDialToProxyWithHandler(https_proxy, nil)
}

func (proxy *ProxyHttpServer) NewConnectDialToProxyWithHandler(https_proxy string, connectReqHandler func(req *http.Request)) func(network, addr string) (net.Conn, error) {
	u, err := url.Parse(https_proxy)
	if err != nil {
		return nil
	}
	if u.Scheme == "" || u.Scheme == "http" {
		if strings.IndexRune(u.Host, ':') == -1 {
			u.Host += ":80"
		}
		return func(network, addr string) (net.Conn, error) {
			connectReq := &http.Request{
				Method: "CONNECT",
				URL: &url.URL{
					Opaque: addr,
				},
				Host:   addr,
				Header: make(http.Header),
			}
			if connectReqHandler != nil {
				connectReqHandler(connectReq)
			}
			c, err := proxy.dial(network, u.Host)
			if err != nil {
				return nil, err
			}
			connectReq.Write(c)
			br := bufio.NewReader(c)
			resp, err := http.ReadResponse(br, connectReq)
			if err != nil {
				c.Close()
				return nil, err
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				resp, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					return nil, err
				}
				c.Close()
				return nil, errors.New("proxy refused connection" + string(resp))
			}
			return c, nil
		}
	}
	if u.Scheme == "https" || u.Scheme == "wss" {
		if strings.IndexRune(u.Host, ':') == -1 {
			u.Host += ":443"
		}
		return func(network, addr string) (net.Conn, error) {
			c, err := proxy.dial(network, u.Host)
			if err != nil {
				return nil, err
			}
			c = tls.Client(c, proxy.Tr.TLSClientConfig)
			connectReq := &http.Request{
				Method: "CONNECT",
				URL:    &url.URL{Opaque: addr},
				Host:   addr,
				Header: make(http.Header),
			}
			if connectReqHandler != nil {
				connectReqHandler(connectReq)
			}
			connectReq.Write(c)
			br := bufio.NewReader(c)
			resp, err := http.ReadResponse(br, connectReq)
			if err != nil {
				c.Close()
				return nil, err
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				body, err := ioutil.ReadAll(io.LimitReader(resp.Body, 500))
				if err != nil {
					return nil, err
				}
				c.Close()
				return nil, errors.New("proxy refused connection" + string(body))
			}
			return c, nil
		}
	}
	return nil
}

func (proxy *ProxyHttpServer) dial(network, addr string) (c net.Conn, err error) {
	if proxy.Tr.Dial != nil {
		return proxy.Tr.Dial(network, addr)
	}
	return net.Dial(network, addr)
}

func (proxy *ProxyHttpServer) connectDial(ctx *ProxyCtx, network, addr string) (c net.Conn, err error) {
	if proxy.ConnectDialWithReq == nil && proxy.ConnectDial == nil {
		return proxy.dial(network, addr)
	}

	if proxy.ConnectDialWithReq != nil {
		return proxy.ConnectDialWithReq(ctx.Req, network, addr)
	}

	return proxy.ConnectDial(network, addr)
}

func TLSConfigFromCA(ca *tls.Certificate) func(host string, ctx *ProxyCtx) (*tls.Config, error) {
	return func(host string, ctx *ProxyCtx) (*tls.Config, error) {
		var err error
		var cert *tls.Certificate

		hostname := stripPort(host)
		config := defaultTLSConfig.Clone()
		ctx.Logf("signing for %s", hostname)

		genCert := func() (*tls.Certificate, error) {
			return signHost(*ca, []string{hostname})
		}
		if ctx.certStore != nil {
			cert, err = ctx.certStore.Fetch(hostname, genCert)
		} else {
			cert, err = genCert()
		}

		if err != nil {
			ctx.Warnf("Cannot sign host certificate with provided CA %s", err)
			return nil, err
		}

		config.Certificates = append(config.Certificates, *cert)
		return config, nil
	}
}
