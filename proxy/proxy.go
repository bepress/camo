package proxy

import (
	"time"

	camourl "github.com/bepress/camo/encoding"
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
		Decoder:             decoder.MustNew(hmacKey),
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

// Proxy implements the handler for proxying assets.
type Proxy struct {
	Decoder decoder.Decoder

	MaxSize        int64
	MaxRedirects   int
	RequestTimeout time.Duration
	ServerName     string

	// TODO(ro) 2017-10-02 Do we really care?
	DisableKeepAlivesBE bool
	DisableKeepAlivesFE bool
}
