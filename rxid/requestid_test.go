package rxid_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bepress/camo/checkers"
	"github.com/bepress/camo/rxid"
)

func TestXIDHandlerCreatesID(t *testing.T) {
	var content = []byte("foo")
	dh := DummyHandler{content}
	h := rxid.Handler(dh)
	rw := httptest.NewRecorder()

	r, err := http.NewRequest("GET", "http://nope", nil)
	checkers.OK(t, err)
	h.ServeHTTP(rw, r)
	checkers.Equals(t, rw.Body.Bytes(), content)
	checkers.Equals(t, len(rw.Header().Get("X-Request-ID")), 20)
}

func TestXIDHandlerPreservesID(t *testing.T) {
	var (
		content = []byte("foo")
		header  = "myheader"
	)

	dh := DummyHandler{content}
	h := rxid.Handler(dh)
	rw := httptest.NewRecorder()

	r, err := http.NewRequest("GET", "http://nope", nil)
	r.Header.Set("X-Request-ID", header)
	checkers.OK(t, err)
	h.ServeHTTP(rw, r)
	checkers.Equals(t, rw.Body.Bytes(), content)
	checkers.Equals(t, rw.Header().Get("X-Request-ID"), header)
}

type DummyHandler struct {
	content []byte
}

func (d DummyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(d.content))
}
