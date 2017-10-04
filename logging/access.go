package logging

import (
	"expvar"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

var (
	proxyCounter *expvar.Map
	// proxyRequests   *expvar.Int
	// proxy400Counter *expvar.Int
	// proxy500Counter *expvar.Int
	// proxyCompleted  *expvar.Int
	// proxyDuration   *expvar.Float

	redirCounter *expvar.Map
	// proxyRequests   *expvar.Int
	// proxy400Counter *expvar.Int
	// proxy500Counter *expvar.Int
	// proxyCompleted  *expvar.Int
	// proxyDuration   *expvar.Float
)

func init() {
	proxyCounter = expvar.NewMap("proxyCounter")
	redirCounter = expvar.NewMap("redirCounter")
}

const (
	// TimeFormat is the time format for logging.
	TimeFormat = time.RFC3339Nano

	err400    = "400"
	err500    = "500"
	requests  = "requests"
	completed = "completed"
	// duration  = "duration"
)

// byteCounter implements an io.Writer wrapping the http.ResponseWriter to
// count bytes written in the response.
type byteCounter struct {
	http.ResponseWriter

	status        int
	responseBytes int64
}

// Write writes bytes to the response writer.
func (ar *byteCounter) Write(b []byte) (int, error) {
	written, err := ar.ResponseWriter.Write(b)
	ar.responseBytes += int64(written)
	return written, err
}

// WriteHeader sets the response status.
func (ar *byteCounter) WriteHeader(status int) {
	ar.status = status
	ar.ResponseWriter.WriteHeader(status)
}

// NewAccessLogger returns a constructed AccessLogger pointer.
func NewAccessLogger(handler http.Handler, logger zerolog.Logger) http.Handler {
	return &AccessLogger{
		handler: handler,
		logger:  logger,
	}
}

// AccessLogger writes an NCSA combined-ish log record. Note this skips the
// rfc931 ident field and username as we aren't supporting either of those
// AFAIU. It also adds an elapsed time, and UTC time to Nanosecond precision
// in RFC3339 format.
type AccessLogger struct {
	handler http.Handler
	logger  zerolog.Logger
}

// ServeHTTP makes our type a http.HandlerFunc.
func (al *AccessLogger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	proxyCounter.Add(requests, 1)
	clientIP := r.RemoteAddr
	if colon := strings.LastIndex(clientIP, ":"); colon != -1 {
		clientIP = clientIP[:colon]
	}
	bc := &byteCounter{
		ResponseWriter: w,
		status:         http.StatusOK,
	}
	start := time.Now()
	al.handler.ServeHTTP(bc, r)
	dur := time.Since(start)
	al.logger.Info().
		Str("client_ip", clientIP).
		Dur("duration", dur).
		Str("domain", r.Host).
		Str("method", r.Method).
		Str("uri", r.RequestURI).
		Str("protocol", r.Proto).
		Int("status", bc.status).
		Int64("reponse_bytes", bc.responseBytes).
		Str("referrer", r.Referer()).
		Str("user_agent", r.UserAgent()).Msg("")

	switch {
	case bc.status == 404:
		proxyCounter.Add("404", 1)
	case bc.status > 399 && bc.status < 500:
		proxyCounter.Add(err400, 1)
	case bc.status > 499:
		proxyCounter.Add(err500, 1)
	}

	proxyCounter.Add(completed, 1)
}