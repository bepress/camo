package main

import "net/http"
import "github.com/bepress/camo/proxy"

func main() {
	p := proxy.MustNew([]byte("test"))
	s := http.Server{Addr: ":8888", Handler: p}
	s.ListenAndServe()
}
