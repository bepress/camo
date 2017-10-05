package main

import (
	"net/http"
	"os"

	"github.com/bepress/camo/proxy"
	"github.com/rs/zerolog"
)

func main() {
	logger := zerolog.New(os.Stderr)
	p := proxy.MustNew([]byte("test"), logger)
	s := http.Server{Addr: ":8888", Handler: p}
	s.ListenAndServe()
}
