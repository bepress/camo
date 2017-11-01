package logging

import (
	"expvar"
	"net/http"
	"strings"
	"time"

	"github.com/bepress/camo/rxid"
	"github.com/paulbellamy/ratecounter"
	"github.com/rs/zerolog"
)

var (
	proxyCounter *expvar.Map
	rps          *ratecounter.RateCounter
	bps          *ratecounter.RateCounter
	durAvg       *ratecounter.AvgRateCounter

	bytesSecond = expvar.NewInt(bytesRate)
	reqSecond   = expvar.NewInt(requestRate)
	avgDuration = expvar.NewFloat(avgDur)
)

func init() {
	proxyCounter = expvar.NewMap("proxyCounter")
	rps = ratecounter.NewRateCounter(time.Second)
	bps = ratecounter.NewRateCounter(time.Second)
	durAvg = ratecounter.NewAvgRateCounter(time.Minute)

}

const (
	// TimeFormat is the time format for logging.
	TimeFormat = time.RFC3339Nano

	avgDur      = "duration_1m_avg"
	bytesRate   = "bytes_second"
	requestRate = "requests_second"

	err400    = "400"
	err404    = "404"
	err500    = "500"
	requests  = "requests"
	completed = "completed"
	bytes     = "bytes_transferred"
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
	logger = logger.With().Str("type", "access").Logger()
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
		Str("request_id", rxid.FromContext(r.Context())).
		Str("client_ip", clientIP).
		Strs("x_forwarded_for", strings.Split(r.Header.Get("X-Forwarded-For"), ", ")).
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
		proxyCounter.Add(err404, 1)
	case bc.status > 399 && bc.status < 500:
		proxyCounter.Add(err400, 1)
	case bc.status > 499:
		proxyCounter.Add(err500, 1)
	}

	proxyCounter.Add(bytes, bc.responseBytes)
	proxyCounter.Add(completed, 1)
	rps.Incr(1)
	bps.Incr(bc.responseBytes)
	durAvg.Incr(dur.Nanoseconds())

	reqSecond.Set(rps.Rate())
	bytesSecond.Set(bps.Rate())
	avgDuration.Set(durAvg.Rate())
}
