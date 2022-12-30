package myproxy_test

import (
	"bytes"
	"crypto/tls"
	"image"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/fj9140/myproxy"
)

var acceptAllCerts = &tls.Config{InsecureSkipVerify: true}
var srv = httptest.NewServer(nil)
var https = httptest.NewTLSServer(nil)
var fs = httptest.NewServer(http.FileServer(http.Dir(".")))

type ConstantHandler string

func (h ConstantHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, string(h))
}

func init() {
	http.DefaultServeMux.Handle("/bobo", ConstantHandler("bobo"))
}

func oneShotProxy(proxy *myproxy.ProxyHttpServer, t *testing.T) (client *http.Client, s *httptest.Server) {
	s = httptest.NewServer(proxy)

	proxyUrl, _ := url.Parse(s.URL)
	tr := &http.Transport{TLSClientConfig: acceptAllCerts, Proxy: http.ProxyURL(proxyUrl)}
	client = &http.Client{Transport: tr}
	return
}

func get(url string, client *http.Client) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	txt, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return nil, err
	}
	return txt, nil
}

func getOrFail(url string, client *http.Client, t *testing.T) []byte {
	txt, err := get(url, client)
	if err != nil {
		t.Fatal("Can't fetch url", url, err)
	}
	return txt
}

func localFile(url string) string {
	return fs.URL + "/" + url
}

func TestSimpleHttpReqWithProxy(t *testing.T) {
	client, s := oneShotProxy(myproxy.NewProxyHttpServer(), t)
	defer s.Close()

	if r := string(getOrFail(srv.URL+"/bobo", client, t)); r != "bobo" {
		t.Error("proxy server does not serve constant handlers", r)
	}
	if string(getOrFail(https.URL+"/bobo", client, t)) != "bobo" {
		t.Error("TLS server does not serve constant handlers, when proxy is used")
	}
}

func TestSimpleHook(t *testing.T) {
	proxy := myproxy.NewProxyHttpServer()
	proxy.OnRequest(myproxy.SrcIpIs("127.0.0.1")).DoFunc(func(req *http.Request, ctx *myproxy.ProxyCtx) (*http.Request, *http.Response) {
		req.URL.Path = "/bobo"
		return req, nil
	})
	client, l := oneShotProxy(proxy, t)
	defer l.Close()

	if result := string(getOrFail(srv.URL+("/momo"), client, t)); result != "bobo" {
		t.Error("Redirecting all requests from 127.0.0.1 to bobo, didn't work." +
			" (Might break if Go's client sets RemoteAddr to IPv6 address). Got: " + result)
	}
}

func TestAlwayHook(t *testing.T) {
	proxy := myproxy.NewProxyHttpServer()
	proxy.OnRequest().DoFunc(func(req *http.Request, ctx *myproxy.ProxyCtx) (*http.Request, *http.Response) {
		req.URL.Path = "/bobo"
		return req, nil
	})
	client, l := oneShotProxy(proxy, t)
	defer l.Close()

	if result := string(getOrFail(srv.URL+("/momo"), client, t)); result != "bobo" {
		t.Error("Redirecting all requests to bobo, didn't work." +
			" (Might break if Go's client sets RemoteAddr to IPv6 address). Got: " + result)
	}
}

func TestReplaceResponse(t *testing.T) {
	proxy := myproxy.NewProxyHttpServer()
	proxy.OnResponse().DoFunc(func(resp *http.Response, ctx *myproxy.ProxyCtx) *http.Response {
		resp.StatusCode = http.StatusOK
		resp.Body = ioutil.NopCloser(bytes.NewBufferString("chico"))
		return resp
	})
	client, l := oneShotProxy(proxy, t)
	defer l.Close()

	if result := string(getOrFail(srv.URL+"/momo", client, t)); result != "chico" {
		t.Error("hooked response, should be chico, instead:", result)
	}
}

func TestReplaceResponseForUrl(t *testing.T) {
	proxy := myproxy.NewProxyHttpServer()
	proxy.OnResponse(myproxy.UrlIs("/koko")).DoFunc(func(resp *http.Response, ctx *myproxy.ProxyCtx) *http.Response {
		resp.StatusCode = http.StatusOK
		resp.Body = ioutil.NopCloser(bytes.NewBufferString("chico"))
		return resp
	})

	client, l := oneShotProxy(proxy, t)
	defer l.Close()

	if result := string(getOrFail(srv.URL+"/koko", client, t)); result != "chico" {
		t.Error("hooked 'koko', should be chico, instead:", result)
	}
	if result := string(getOrFail(srv.URL+"/bobo", client, t)); result != "bobo" {
		t.Error("still, bobo should stay as usual, instead:", result)
	}
}

func TestOneShotFileServer(t *testing.T) {
	client, l := oneShotProxy(myproxy.NewProxyHttpServer(), t)
	defer l.Close()

	file := "test_data/panda.png"
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal("Cannot find ", file)
	}
	if resp, err := client.Get(fs.URL + "/" + file); err == nil {
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal("got", string(b))
		}
		if int64(len(b)) != info.Size() {
			t.Error("Expected Length", file, info.Size(), "actually", len(b), "starts", string(b[:10]))
		}
	} else {
		t.Fatal("Cannot read from fs server", err)
	}
}

func TestContentType(t *testing.T) {
	proxy := myproxy.NewProxyHttpServer()
	proxy.OnResponse(myproxy.ContentTypeIs("image/png")).DoFunc(func(resp *http.Response, ctx *myproxy.ProxyCtx) *http.Response {
		resp.Header.Set("X-Shmoopi", "1")
		return resp
	})

	client, l := oneShotProxy(proxy, t)
	defer l.Close()

	for _, file := range []string{"test_data/panda.png", "test_data/football.png"} {
		if resp, err := client.Get(localFile(file)); err != nil || resp.Header.Get("X-Shmoopi") != "1" {
			if err == nil {
				t.Error("pngs should have X-Shmoopi header = 1, actually", resp.Header.Get("X-Shmoopi"))
			} else {
				t.Error("error reading png", err)
			}
		}
	}

	file := "baby.jpg"
	if resp, err := client.Get(localFile(file)); err != nil || resp.Header.Get("X-Shmoopi") != "" {
		if err == nil {
			t.Error("Non png images should NOT have X-Shmoopi header at all", resp.Header.Get("X-Shmoopi"))
		} else {
			t.Error("error reading png", err)
		}
	}
}

func getImage(file string, t *testing.T) image.Image {
	newimage, err := ioutil.ReadFile(file)
	if err != nil {
		t.Fatal("Cannot read file", file, err)
	}
	img, _, err := image.Decode(bytes.NewReader(newimage))
	if err != nil {
		t.Fatal("Cannot decode image", file, err)
	}
	return img
}

func TestConstantImageHandler(t *testing.T) {
	proxy := myproxy.NewProxyHttpServer()

}
