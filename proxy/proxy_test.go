package proxy_test

import (
	"testing"

	"github.com/bepress/camo/checkers"
	"github.com/bepress/camo/proxy"
)

func TestProxyPanicsNilKey(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("proxy.MustNew failed to panic")
		}
	}()

	_ = proxy.MustNew(nil)
}

func TestProxyPanicsEmptyKey(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("proxy.MustNew failed to panic")
		}
	}()

	_ = proxy.MustNew([]byte(""))
}

func TestDefaultProxyValues(t *testing.T) {
	tut := proxy.MustNew([]byte("test"))

	checkers.Equals(t, tut.MaxRedirects, 10)
	checkers.Equals(t, tut.MaxSize, int64(5*1024*1024))
}

func TestProxyWithOptions(t *testing.T) {
	tut := proxy.MustNew([]byte("test"),
		func(p *proxy.Proxy) { p.MaxRedirects = 1 },
		func(p *proxy.Proxy) { p.Decoder = DummyDecoder{url: "http://example.com/someurl"} })

	checkers.Equals(t, tut.MaxRedirects, 1)

	got, err := tut.Decoder.Decode("ignored", "input")
	checkers.OK(t, err)
	checkers.Equals(t, got, "http://example.com/someurl")
}

type DummyDecoder struct {
	err error
	url string
}

func (dd DummyDecoder) Decode(_, _ string) (string, error) {
	if dd.err != nil {
		return "", dd.err
	}
	return dd.url, nil
}
