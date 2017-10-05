package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bepress/camo/decoder"
	"github.com/bepress/camo/filter"
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
func MustNew(hmacKey []byte, options ...func(*Proxy)) *Proxy {
	if len(hmacKey) == 0 {
		panic("hmacKey must not be nil or empty")
	}

	p := &Proxy{
		// Change the constructor name
		Decoder: decoder.MustNew(hmacKey),
		Filter:  filter.MustNewCIDR(FilteredIPNetworks),

		LookupIP: net.LookupIP,

		MaxSize:             DefaultMaxSize,
		MaxRedirects:        DefaultMaxRedirects,
		RequestTimeout:      DefaultRequestTimeout,
		ServerName:          DefaultServerName,
		DisableKeepAlivesBE: DefaultKABE,
		DisableKeepAlivesFE: DefaultKAFE,
	}

	for _, opt := range options {
		opt(p)
	}

	return p
}

// ResolverFunc is the net.LookupIP signature so we can fake it in tests.
type ResolverFunc func(string) ([]net.IP, error)

// Proxy implements the handler for proxying assets.
type Proxy struct {
	Decoder  decoder.Decoder
	Filter   *filter.CIDRFilter
	LookupIP ResolverFunc

	MaxSize        int64
	MaxRedirects   int
	RequestTimeout time.Duration
	ServerName     string

	// TODO(ro) 2017-10-02 Do we really care?
	DisableKeepAlivesBE bool
	DisableKeepAlivesFE bool
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.setHeaders(w, r)

	if r.Method != "GET" && r.Method != "HEAD" {
		w.Header().Add("Allowed", "GET,HEAD")
		http.Error(w, fmt.Sprintf("Method not allowed: %s", r.Method), http.StatusMethodNotAllowed)
	}

	if r.Header.Get("Via") == p.ServerName {
		http.Error(w, "Redirect loop detected", http.StatusNotFound)
		return
	}

	// Split path and get components.
	sig, encodedURL, err := p.splitComponents(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Decode the URL.
	uStr, err := p.Decoder.Decode(sig, encodedURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	// Validate the target host
	if err = p.validateTarget(uStr); err != nil {
		http.Error(w, "invalid host: "+err.Error(), http.StatusBadRequest)
		return
	}

	client := http.Client{Timeout: p.RequestTimeout}
	req, err := http.NewRequest(r.Method, uStr, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		if resp == nil {
			http.Error(w, fmt.Sprintf("error processing request: %q", err), http.StatusBadRequest)
			return
		}
	}

	// This is throwaway when we're done
	fmt.Fprint(w, uStr)
}

func (p *Proxy) validateTarget(uStr string) error {
	// filter out rejected networks
	u, err := url.Parse(uStr)
	if err != nil {
		return err
	}
	ips, err := p.LookupIP(u.Host)
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
		// TODO(ro) 2017-10-04 Do we want to use this too?
		if !ip.IsGlobalUnicast() {
			return fmt.Errorf("resolved to reserved address: %q", ip)
		}

	}

	return nil
}

func (p *Proxy) setHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Via", p.ServerName)
	if p.DisableKeepAlivesFE {
		w.Header().Set("Connection", "close")
	}
}

func (p *Proxy) splitComponents(path string) (string, string, error) {
	parts := strings.Split(path, "/")
	if len(parts) != 3 {
		return "", "", fmt.Errorf("invalid camo url path: %s, wanted 3 parts got %d", path, len(parts))
	}
	return parts[1], parts[2], nil
}
