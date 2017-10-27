package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	stdlog "log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	_ "expvar"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/bepress/camo/helpers"
	"github.com/bepress/camo/logging"
	"github.com/bepress/camo/proxy"
	"github.com/reedobrien/cowl"
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
		addr        = flag.String("addr", ":443", "The address and port to listen on")
		flushPeriod = flag.Duration("flushPeriod", 10*time.Second, "The maximum period to wait before flushing")
		flushSize   = flag.Int("flushSize", 7000, "The maximum size the log buffer may reach before flushing")
		maxsize     = flag.Int64("maxsize", 5, "Maximum size to proxy in whole MB (no decimal)")
		secret      = flag.String("secret", "", "The 'shared secret' hmac key")
		tlscert     = flag.String("cert", "cert.pem", "The TLS certificate to use")
		tlskey      = flag.String("key", "key.pem", "The TLS key to use")
		verbose     = flag.Bool("verbose", false, "If verbose logging should take place (No-op at this time as there's no debug log statements)")
		version     = flag.Bool("version", false, "Display version and build info, then exit")

		// TODO(ro) 2017-10-10 Add flags for other proxy set-ables.

		logger zerolog.Logger
		hmac   string
	)
	flag.Parse()

	if *version {
		versionInfo()
	}

	// Create a context so we can cancel goroutines.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Make sure they go away when main exits.

	hostname, err := os.Hostname()
	if err != nil {
		panic("failed to get hostname")
	}

	// Start an aws session, and make the cloudwatchlogs service.
	session := session.Must(session.NewSession())
	svc := cloudwatchlogs.New(session)

	// Create the cloudwatchlog writer.
	cwlOpts := []func(*cowl.Writer){
		func(w *cowl.Writer) { w.FlushPeriod = *flushPeriod },
		func(w *cowl.Writer) { w.FlushSize = *flushSize },
	}
	cwl := cowl.MustNewWriterWithContext(ctx, svc, app, "app-"+hostname, cwlOpts...)

	// Write to both stdout and cwl.
	w := io.MultiWriter(cwl, os.Stdout)

	// Set up the logger to use it.
	logger = logging.NewLogger(app, *verbose, w).With().Str(
		"handler", "proxy").Logger()
	if BuildVersion != "" {
		logger = logger.With().
			Str("app_version", BuildVersion+"-"+GitHash).
			Logger()
	}

	stdlog.SetFlags(0)
	stdlog.SetOutput(logger)

	// Stop channel to block until we get a signal. This is used for graceful
	// shutdown.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Set up options.
	// TODO(ro) 2017-10-11 Add more options here and as flags as necessary.
	options := []func(*proxy.Proxy){}
	if *maxsize > 0 {
		options = append(options, func(p *proxy.Proxy) { p.MaxSize = *maxsize * 1024 * 1024 })
	}

	// Create proxy handler.
	hmac = helpers.GetHMAC(*secret)
	p := proxy.MustNew([]byte(hmac), logger, options...)
	// Wrap proxy handler with logger.
	proxyHandler := logging.NewAccessLogger(p, logger)
	s := http.Server{
		Addr:         *addr,
		Handler:      proxyHandler,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start TLS server.
	go func() {
		if err := s.ListenAndServeTLS(*tlscert, *tlskey); err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("failed to start server")
		}
	}()
	// Start a server on localhost:9000 for expvar.
	go func() {
		if err := http.ListenAndServe("127.0.0.1:9000", nil); err != nil {
			logger.Fatal().Err(err).Msg("failed to start expvar server")
		}
	}()

	<-stop

	// Create a context with timeout to limit shutdown time in the event of
	// slow clients.
	ctx, cancel = context.WithTimeout(ctx, drainTime)
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
