package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/fj9140/myproxy"
)

func main() {
	verbos := flag.Bool("v", false, "should every proxy request be logged to stdout")
	addr := flag.String("addr", ":8080", "proxy listen address")
	flag.Parse()
	proxy := myproxy.NewProxyHttpServer()
	proxy.Verbose = *verbos
	log.Fatal(http.ListenAndServe(*addr, proxy))

}
