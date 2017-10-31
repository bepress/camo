package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/bepress/camo/decoder"
	"github.com/bepress/camo/filter"
	"github.com/reedobrien/rbp"
	"github.com/rs/zerolog"
)

const (
	// DefaultMaxSize is the maximum size we'll proxy.
	DefaultMaxSize = 5 * 1024 * 1024

	// DefaultMaxRedirects is the maximum # of redirects we'll follow.
	DefaultMaxRedirects = 10

	// DefaultRequestTimeout is the request timeout for the client.
	DefaultRequestTimeout = 4 * time.Second

	// DefaultServerName is a name to set our client and Via header to.
	DefaultServerName = "bepress/camo"

	//DefaultKABE is the default keepalives setting for Backends.
	DefaultKABE = false

	//DefaultKAFE is the default keepalives setting for Frontends.
	DefaultKAFE = false
)

// MustNew returns a Proxy handler or panics.
func MustNew(hmacKey []byte, logger zerolog.Logger, options ...func(*Proxy)) *Proxy {
	if len(hmacKey) == 0 {
		panic("hmacKey must not be nil or empty")
	}

	p := &Proxy{
		BufferPool:          rbp.NewBufferPool(),
		CheckUnicast:        true,
		Decoder:             decoder.MustNew(hmacKey),
		DisableKeepAlivesBE: DefaultKABE,
		DisableKeepAlivesFE: DefaultKAFE,
		Filter:              filter.MustNewCIDR(FilteredIPNetworks),
		LookupIP:            net.LookupIP,
		MaxRedirects:        DefaultMaxRedirects,
		MaxSize:             DefaultMaxSize,
		RequestTimeout:      DefaultRequestTimeout,
		ServerName:          DefaultServerName,

		logger: logger,
	}

	for _, opt := range options {
		opt(p)
	}

	if p.Transport == nil {

		p.Transport = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   3 * time.Second,
				KeepAlive: 30 * time.Second}).DialContext,
			// Timeouts
			ExpectContinueTimeout: 1 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			TLSHandshakeTimeout:   3 * time.Second,
			IdleConnTimeout:       30 * time.Second,

			MaxIdleConns:        300,
			MaxIdleConnsPerHost: 8,
			DisableKeepAlives:   p.DisableKeepAlivesBE,
		}
	}

	p.client = &http.Client{
		Transport: p.Transport,
		Timeout:   p.RequestTimeout,
	}
	if p.RedirFunc == nil {
		p.RedirFunc = p.checkRedirect
	}
	p.client.CheckRedirect = p.RedirFunc

	return p
}

// CheckRedirect implments the redirect manager for http.Client
func (p *Proxy) checkRedirect(r *http.Request, via []*http.Request) error {
	if err := p.validateTarget(r.URL); err != nil {
		return err
	}

	if len(via) >= p.MaxRedirects {
		return fmt.Errorf("stopped after %d redirects", p.MaxRedirects)
	}
	return nil

}

// ResolverFunc is the net.LookupIP signature so we can fake it in tests.
type ResolverFunc func(string) ([]net.IP, error)

// Proxy implements the handler for proxying assets.
type Proxy struct {
	BufferPool     httputil.BufferPool
	CheckUnicast   bool
	Decoder        decoder.Decoder
	Filter         *filter.CIDRFilter
	FlushInterval  time.Duration
	LookupIP       ResolverFunc
	MaxRedirects   int
	MaxSize        int64
	RedirFunc      func(*http.Request, []*http.Request) error
	RequestTimeout time.Duration
	ServerName     string
	Transport      http.RoundTripper
	client         *http.Client
	logger         zerolog.Logger

	// TODO(ro) 2017-10-02 Do we really care?
	DisableKeepAlivesBE bool
	DisableKeepAlivesFE bool
}

// ServeHTTP implements HandlerFunc.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.setResponseHeaders(w)

	if r.Method != "GET" && r.Method != "HEAD" {
		w.Header().Add("Allowed", "GET,HEAD")
		http.Error(w, fmt.Sprintf("Method not allowed: %s", r.Method), http.StatusMethodNotAllowed)
		return
	}

	if r.Header.Get("Via") == p.ServerName {
		http.Error(w, "Redirect loop detected", http.StatusNotFound)
		return
	}

	// Split path and get components.
	sig, encodedURL, err := p.splitComponents(r.URL.Path)
	if err != nil {
		// If it is not a valid signed URL, it may be a health check for the
		// ELB. Handle that here.
		if r.URL.Path == "/health" {
			fmt.Fprintln(w, "OK")
			return
		}
		if r.URL.Path == "/favicon.ico" {
			w.Header().Set("Expires", time.Now().UTC().AddDate(0, 1, 0).Format(http.TimeFormat))
			w.Write(bepressFavicon)
		}
		p.logger.Error().Err(err).Msg(errDetails())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Decode the URL.
	uStr, err := p.Decoder.Decode(sig, encodedURL)
	if err != nil {
		p.logger.Error().Err(err).Msg(errDetails())
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	u, err := url.Parse(uStr)
	if err != nil {
		p.logger.Error().Err(err).Msg(errDetails())
		http.Error(w, "Invalid downstream URL: "+err.Error(), http.StatusForbidden)
		return
	}

	// Validate the target host
	if err = p.validateTarget(u); err != nil {
		p.logger.Error().Err(err).Msg(errDetails())
		http.Error(w, "invalid host: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Build the request for downstream.
	outreq, err := p.buildRequest(u, w, r)
	if err != nil {
		p.logger.Error().Err(err).Msg(errDetails())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Perform the request.
	resp, err := p.client.Do(outreq)
	if err != nil {
		p.logger.Error().Err(err).Msg(errDetails())
		http.Error(w, fmt.Sprintf("error processing request: %q", err), http.StatusInternalServerError)
		return
	}

	// Log information about the upstream request/response.
	p.logger.Info().
		Str("upstream_domain", outreq.Host).
		Int("upstream_response", resp.StatusCode).
		Str("upstream_path", outreq.URL.Path).
		Str("content_type", resp.Header.Get("Content-Type")).
		Int64("content_length", resp.ContentLength).Msg("")

	defer resp.Body.Close()

	if resp.ContentLength > p.MaxSize {
		http.Error(w, "Payload too large", http.StatusRequestEntityTooLarge)
		return
	}

	switch resp.StatusCode {
	case 200, 206, 304, 410:
		p.buildResponse(w, resp)
		return
	case 301, 302, 303, 307:
		http.Error(w, "Too many redirects", http.StatusNotFound)
		return
	case 500, 502, 503, 504:
		http.Error(w, "Error Fetching Resource: "+resp.Status, http.StatusBadGateway)
		return
	default:
		http.Error(w, "Unable to find suitable content", http.StatusNotFound)
		return
	}
}

// buildResponse massages the headers on the response to the upstream request.
func (p *Proxy) buildResponse(outbound http.ResponseWriter, inbound *http.Response) {
	// Remove hop-by-hop headers listed in the
	// "Connection" header of the response.
	if c := inbound.Header.Get("Connection"); c != "" {
		for _, f := range strings.Split(c, ",") {
			if f = strings.TrimSpace(f); f != "" {
				inbound.Header.Del(f)
			}
		}
	}

	for _, h := range hopHeaders {
		inbound.Header.Del(h)
	}

	copyHeader(outbound.Header(), inbound.Header)

	// The "Trailer" header isn't included in the Transport's response,
	// at least for *http.Transport. Build it up from Trailer.
	announcedTrailers := len(inbound.Trailer)
	if announcedTrailers > 0 {
		trailerKeys := make([]string, 0, len(inbound.Trailer))
		for k := range inbound.Trailer {
			trailerKeys = append(trailerKeys, k)
		}
		outbound.Header().Add("Trailer", strings.Join(trailerKeys, ", "))
	}

	outbound.WriteHeader(inbound.StatusCode)

	if len(inbound.Trailer) > 0 {
		// Force chunking if we saw a response trailer.
		// This prevents net/http from calculating the length for short
		// bodies and adding a Content-Length.
		if fl, ok := outbound.(http.Flusher); ok {
			fl.Flush()
		}
	}
	p.copyResponse(outbound, inbound.Body)
	inbound.Body.Close()

	for k, vv := range inbound.Trailer {
		k = http.TrailerPrefix + k
		for _, v := range vv {
			outbound.Header().Add(k, v)
		}
	}

}

// buildRequest builds the request for the upstream resource. It creates the
// request, copies/adds and filters headers.
func (p *Proxy) buildRequest(target *url.URL, w http.ResponseWriter, r *http.Request) (*http.Request, error) {

	ctx := r.Context()
	// Listen on the close notifier and setup a context which we can cancel if
	// we receive close notification (i.e. the client went away) or if we
	// timeout.
	if cn, ok := w.(http.CloseNotifier); ok {
		var cancel context.CancelFunc
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		notifyCh := cn.CloseNotify()
		go func() {
			select {
			case <-time.After(p.RequestTimeout):
				cancel()
			case <-notifyCh:
				cancel()
			case <-ctx.Done():
			}
		}()
	}

	// Copy the request with our context.
	out := r.WithContext(ctx)
	if r.ContentLength == 0 {
		// Apparently we need to set the body to nil to get Transport retries for http/1.1.
		// See https://github.com/golang/go/issues/16036 and
		// https://github.com/golang/go/issues/13722
		out.Body = nil
	}
	// Set the URL to the decoded target
	out.URL = target
	out.Host = out.URL.Host
	out.Header = cloneHeader(r.Header) // r.WithContext does shallow copies.

	// Remove hop-by-hop headers listed in the "Connection" header.
	// See RFC 2616, section 14.10.
	if c := out.Header.Get("Connection"); c != "" {
		for _, f := range strings.Split(c, ",") {
			if f = strings.TrimSpace(f); f != "" {
				out.Header.Del(f)
			}
		}
	}

	// Remove hop-by-hop headers to the backend.
	for _, h := range hopHeaders {
		if out.Header.Get(h) != "" {
			out.Header.Del(h)
		}
	}

	if clientIP, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		// If we aren't the first proxy retain prior
		// X-Forwarded-For information as a comma+space
		// separated list and fold multiple headers into one.
		if prior, ok := out.Header["X-Forwarded-For"]; ok {
			clientIP = strings.Join(prior, ", ") + ", " + clientIP
		}
		out.Header.Set("X-Forwarded-For", clientIP)
	}

	// Backwards... because we always expect https.
	var proto = "https"
	if r.TLS == nil {
		proto = "http"
	}
	out.Header.Set("X-Forwarded-Proto", proto)

	// Because we copied the incoming upstream request (server's view) we need
	// to remove the RequestURI, which represents 'Request-Line' in the
	// original request to our server.
	out.RequestURI = ""

	// TODO(ro) 2017-10-21 Can we actually return an error here? I don't see it in the current read through. If not remove it from the function signature.
	return out, nil
}

// validateTarget filters against known invalid networks. And then checks to
// ensure is is a global unicast address.
func (p *Proxy) validateTarget(u *url.URL) error {
	// filter out rejected networks
	host := strings.Split(u.Host, ":")[0]
	ips, err := p.LookupIP(host)
	if err != nil {
		return err
	}
	for _, ip := range ips {
		allowed, err := p.Filter.Allowed(ip.String())
		if err != nil {
			return fmt.Errorf("error resolving host target(%q): %q", ip, err)
		}
		if !allowed {
			return fmt.Errorf("filtered host address: %q", ip)
		}
		if p.CheckUnicast {
			// TODO(ro) 2017-10-04 Do we want to use this too?
			if !ip.IsGlobalUnicast() {
				return fmt.Errorf("resolved to reserved address: %q", ip)
			}
		}
	}

	return nil
}

// setResponseHeaders sets headers on our outgoing response. The Via header so
// we can catch redirect loops. Connection: close to disable keepalives.  I
// suppose we may wan this if we have a spike in traffic from very slow
// clients. Also, some securuity headers.
// TODO(ro) 2017-10-06 Revisit CSP Header.
func (p *Proxy) setResponseHeaders(w http.ResponseWriter) {
	w.Header().Set("Via", p.ServerName)
	if p.DisableKeepAlivesFE {
		w.Header().Set("Connection", "close")
	}

	for _, h := range addRespHeaders {
		w.Header().Set(h.key, h.val)
	}

}

// splitComponents splits the incoming path and verifies the shape and size.
func (p *Proxy) splitComponents(path string) (string, string, error) {
	parts := strings.Split(path, "/")
	if len(parts) != 3 {
		return "", "", fmt.Errorf("invalid camo url path: %s, wanted 3 parts got %d", path, len(parts))
	}
	return parts[1], parts[2], nil
}

func (p *Proxy) copyResponse(dst io.Writer, src io.Reader) {
	if p.FlushInterval != 0 {
		if wf, ok := dst.(writeFlusher); ok {
			mlw := &maxLatencyWriter{
				dst:     wf,
				latency: p.FlushInterval,
				done:    make(chan bool),
			}

			go mlw.flushLoop()
			defer mlw.stop()
			dst = mlw
		}
	}

	buf := p.BufferPool.Get()
	p.copyBuffer(dst, src, buf)
	p.BufferPool.Put(buf)
}

func (p *Proxy) copyBuffer(dst io.Writer, src io.Reader, buf []byte) (int64, error) {

	if len(buf) == 0 {
		buf = make([]byte, 32*1024)
	}

	var written int64
	for {
		nr, rerr := src.Read(buf)
		if rerr != nil && rerr != io.EOF && rerr != context.Canceled {
			p.logger.Error().Err(rerr).Msgf(
				"Proxy read error during body copy: %v", rerr)
		}
		if nr > 0 {
			nw, werr := dst.Write(buf[:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if werr != nil {
				return written, werr
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if rerr != nil {
			return written, rerr
		}
	}
}

// OnExitFlushLoop is a callback set by tests to detect the state of the
// flushLoop() goroutine.
var OnExitFlushLoop func()

type writeFlusher interface {
	io.Writer
	http.Flusher
}

type maxLatencyWriter struct {
	dst     writeFlusher
	latency time.Duration

	mu   sync.Mutex // protects Write + Flush
	done chan bool
}

// Write implements io.Writer
func (m *maxLatencyWriter) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.dst.Write(p)
}

func (m *maxLatencyWriter) flushLoop() {
	t := time.NewTicker(m.latency)
	defer t.Stop()

	for {
		select {
		case <-m.done:
			if OnExitFlushLoop != nil {
				OnExitFlushLoop()
			}
			return
		case <-t.C:
			m.mu.Lock()
			m.dst.Flush()
			m.mu.Unlock()
		}
	}
}

func (m *maxLatencyWriter) stop() { m.done <- true }

func errDetails() string {
	pc, fn, line, _ := runtime.Caller(1)
	return fmt.Sprintf("[error] in %s[%s:%d]", runtime.FuncForPC(pc).Name(), fn, line)
}
