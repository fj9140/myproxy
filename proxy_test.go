package myproxy_test

import (
	"crypto/tls"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/fj9140/myproxy"
)

var acceptAllCerts = &tls.Config{InsecureSkipVerify: true}
var srv = httptest.NewServer(nil)
var https = httptest.NewTLSServer(nil)

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
