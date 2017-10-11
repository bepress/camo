package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	_ "expvar"

	"github.com/bepress/camo/logging"
	"github.com/bepress/camo/proxy"
	"github.com/rs/zerolog"
)

const (
	app       = "camo"
	drainTime = 10 * time.Second
)

var (
	// BuildDate is populated on build for version info.
	BuildDate string

	// GitBranch is populated on build for version info.
	GitBranch string

	// GitHash is populated on build for version info.
	GitHash string

	// BuildVersion is populated on build for version info.
	BuildVersion string
)

func main() {
	var (
		addr    = flag.String("addr", ":443", "The address and port to listen on")
		maxsize = flag.Int64("maxsize", 5, "Maximum size to proxy in whole MB (no decimal)")
		secret  = flag.String("secret", "", "The 'shared secret' hmac key")
		tlscert = flag.String("cert", "cert.pem", "The TLS certificate to use")
		tlskey  = flag.String("key", "key.pem", "The TLS key to use")
		verbose = flag.Bool("verbose", false, "If verbose logging should take place (No-op at this time)")
		version = flag.Bool("version", false, "Display version and build info, then exit")

		// TODO(ro) 2017-10-10 Add flags for other proxy set-ables.
		logger zerolog.Logger
	)
	flag.Parse()

	if *version {
		versionInfo()
	}

	logger = logging.NewLogger(app, *verbose, nil).With().Str(
		"handler", "proxy").Logger()
	if BuildVersion != "" {
		logger = logger.With().
			Str("app_version", BuildVersion+"-"+GitHash).
			Logger()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	p := proxy.MustNew([]byte(*secret), logger, func(p *proxy.Proxy) { p.MaxSize = *maxsize * 1024 * 1024 })
	proxyHandler := logging.NewAccessLogger(p, logger)
	s := http.Server{Addr: *addr, Handler: proxyHandler}
	go func() {
		if err := s.ListenAndServeTLS(*tlscert, *tlskey); err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("failed to start server")
		}
	}()
	go func() {
		if err := http.ListenAndServe("127.0.0.1:9000", nil); err != nil {
			logger.Fatal().Err(err).Msg("failed to start expvar server")
		}
	}()

	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), drainTime)
	defer cancel()

	if err := s.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("caught on proxy.Shutdown")
	}
}

func versionInfo() {
	fmt.Printf(`
build date: %s
branch    : %s
hash      : %s
version   : %s
built with: %s
`[1:], BuildDate, GitBranch, GitHash, BuildVersion, runtime.Version())
	os.Exit(0)
}
