package proxy_test

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bepress/camo/checkers"
	"github.com/bepress/camo/filter"
	"github.com/bepress/camo/proxy"
	"github.com/rs/zerolog"
)

func TestProxyPanicsNilKey(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("proxy.MustNew failed to panic")
		}
	}()

	_ = proxy.MustNew(nil, zerolog.New(ioutil.Discard))
}

func TestProxyPanicsEmptyKey(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("proxy.MustNew failed to panic")
		}
	}()

	_ = proxy.MustNew([]byte(""), zerolog.New(ioutil.Discard))
}

func TestDefaultProxyValues(t *testing.T) {
	tut := proxy.MustNew([]byte("test"), zerolog.New(ioutil.Discard))

	checkers.Equals(t, tut.MaxRedirects, 10)
	checkers.Equals(t, tut.MaxSize, int64(5*1024*1024))
}

func TestProxyWithOptions(t *testing.T) {
	resolver := DummyResolver{ips: []net.IP{
		net.ParseIP("72.5.9.223"),
	}}

	tut := proxy.MustNew([]byte("test"),
		zerolog.New(ioutil.Discard),
		func(p *proxy.Proxy) { p.MaxRedirects = 1 },
		func(p *proxy.Proxy) { p.LookupIP = resolver.LookupIP },
		func(p *proxy.Proxy) { p.Decoder = DummyDecoder{url: "http://example.com/someurl"} })

	checkers.Equals(t, tut.MaxRedirects, 1)

	got, err := tut.Decoder.Decode("ignored", "input")
	checkers.OK(t, err)
	checkers.Equals(t, got, "http://example.com/someurl")
}

func TestAllowedMethods(t *testing.T) {
	table := []struct {
		signedURI   string
		method      string
		wantCode    int
		wantHeader  string
		unSignedURI string
	}{
		{"/sYH2aI8SV7JV_KpxO2zgGetvJbw/aHR0cDovL3Rlc3RpbmcuYmVwcmVzcy5jb20vc2lkX2dhbGxlcnlfb25lLzEwMTkvcHJldmlldy5qcGc", "GET", 200, "", "/sid_gallery_one/1019/preview.jpg"},
		{"/sYH2aI8SV7JV_KpxO2zgGetvJbw/aHR0cDovL3Rlc3RpbmcuYmVwcmVzcy5jb20vc2lkX2dhbGxlcnlfb25lLzEwMTkvcHJldmlldy5qcGc", "HEAD", 200, "", "/sid_gallery_one/1019/preview.jpg"},
		{"/sYH2aI8SV7JV_KpxO2zgGetvJbw/aHR0cDovL3Rlc3RpbmcuYmVwcmVzcy5jb20vc2lkX2dhbGxlcnlfb25lLzEwMTkvcHJldmlldy5qcGc", "PUT", 405, "GET,HEAD", "/sid_gallery_one/1019/preview.jpg"},
		{"/sYH2aI8SV7JV_KpxO2zgGetvJbw/aHR0cDovL3Rlc3RpbmcuYmVwcmVzcy5jb20vc2lkX2dhbGxlcnlfb25lLzEwMTkvcHJldmlldy5qcGc", "POST", 405, "GET,HEAD", "/sid_gallery_one/1019/preview.jpg"},
		{"/sYH2aI8SV7JV_KpxO2zgGetvJbw/aHR0cDovL3Rlc3RpbmcuYmVwcmVzcy5jb20vc2lkX2dhbGxlcnlfb25lLzEwMTkvcHJldmlldy5qcGc", "DELETE", 405, "GET,HEAD", "/sid_gallery_one/1018/preview.jpg"},
	}

	resolver := DummyResolver{ips: []net.IP{
		net.ParseIP("72.5.9.223"),
	}}

	hmacKey := []byte("test")

	tut := proxy.MustNew(
		hmacKey,
		zerolog.New(ioutil.Discard),
		func(p *proxy.Proxy) {
			p.Decoder = DummyDecoder{url: "http://fail"} // Set later
		},
		func(p *proxy.Proxy) { p.LookupIP = resolver.LookupIP },
	)

	// The test camo server which decodes and fetches from backend.
	ts := httptest.NewTLSServer(tut)

	client := ts.Client()
	// Disable compression since we don't set it up on the BE.
	client.Transport.(*http.Transport).DisableCompression = true

	// The backend server that we proxy to/from.
	tsBE := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, "/sid_gallery_one/1019/preview.jpg") }))

	for _, test := range table {
		tut.Decoder = DummyDecoder{url: tsBE.URL + test.unSignedURI}
		req, err := http.NewRequest(test.method, ts.URL+test.signedURI, nil)
		checkers.OK(t, err)

		resp, err := client.Do(req)
		checkers.OK(t, err)

		checkers.Equals(t, resp.StatusCode, test.wantCode)
		checkers.Equals(t, resp.Header.Get("Allowed"), test.wantHeader)

		if test.method == "GET" {
			got, err := ioutil.ReadAll(resp.Body)
			checkers.OK(t, err)
			resp.Body.Close()
			checkers.Equals(t, string(got), test.unSignedURI)
		}
	}
}

func TestExpectedHeaders(t *testing.T) {
	testName := "testserver"
	type h struct {
		key   string
		value string
	}
	via := h{key: "Via", value: testName}
	// Connection is removed either by the test server or client.  However, we
	// can see the header.Set call is covered. So we'll live with that for now.
	// kafe := h{key: "Connection", value: "close"}

	table := []struct {
		uri       string
		disableFE bool
	}{
		{"/sYH2aI8SV7JV_KpxO2zgGetvJbw/aHR0cDovL3Rlc3RpbmcuYmVwcmVzcy5jb20vc2lkX2dhbGxlcnlfb25lLzEwMTkvcHJldmlldy5qcGc", false},
		{"/sYH2aI8SV7JV_KpxO2zgGetvJbw/aHR0cDovL3Rlc3RpbmcuYmVwcmVzcy5jb20vc2lkX2dhbGxlcnlfb25lLzEwMTkvcHJldmlldy5qcGc", true},
	}

	resolver := DummyResolver{ips: []net.IP{
		net.ParseIP("72.5.9.223"),
		net.ParseIP("2606:2800:220:1:248:1893:25c8:1946"),
	}}

	for _, test := range table {
		tut := proxy.MustNew(
			[]byte("test"),
			zerolog.New(ioutil.Discard),
			func(p *proxy.Proxy) { p.DisableKeepAlivesFE = test.disableFE },
			func(p *proxy.Proxy) { p.ServerName = testName },
			func(p *proxy.Proxy) { p.LookupIP = resolver.LookupIP },
			func(p *proxy.Proxy) {
				p.Decoder = DummyDecoder{url: "http://example.com/someurl"}
			},
		)
		ts := httptest.NewTLSServer(tut)
		client := ts.Client()
		resp, err := client.Get(ts.URL + test.uri)
		checkers.OK(t, err)
		checkers.Equals(t, resp.Header.Get(via.key), via.value)
	}
}

func TestRedirectLoop(t *testing.T) {
	testName := "testserver"
	resolver := DummyResolver{ips: []net.IP{
		net.ParseIP("72.5.9.223"),
	}}
	tut := proxy.MustNew(
		[]byte("test"),
		zerolog.New(ioutil.Discard),
		func(p *proxy.Proxy) { p.ServerName = testName },
		func(p *proxy.Proxy) { p.LookupIP = resolver.LookupIP },
		func(p *proxy.Proxy) {
			p.Decoder = DummyDecoder{url: "http://example.com/someurl"}
		},
	)
	ts := httptest.NewTLSServer(tut)
	client := ts.Client()
	req, err := http.NewRequest("GET", ts.URL+"/sig/url", nil)
	checkers.OK(t, err)

	req.Header.Set("Via", testName)
	resp, err := client.Do(req)
	checkers.OK(t, err)
	checkers.Equals(t, resp.StatusCode, http.StatusNotFound)

	got, err := ioutil.ReadAll(resp.Body)
	checkers.OK(t, err)
	resp.Body.Close()
	checkers.Equals(t, string(got), "Redirect loop detected\n")
}

func TestInvalidPathOrURL(t *testing.T) {
	table := []struct {
		uri      string
		wantCode int
		wantMsg  string
	}{
		{"/I2s_jHIbZkwmHHX8wb8/hmdxDM1g/aH0cDovL2JlcHJlc3MuY29t",
			http.StatusBadRequest,
			"invalid camo url path: /I2s_jHIbZkwmHHX8wb8/hmdxDM1g/aH0cDovL2JlcHJlc3MuY29t, wanted 3 parts got 4\n"},
		{"/one",
			http.StatusBadRequest,
			"invalid camo url path: /one, wanted 3 parts got 2\n"},
		{"/two/withtrailingslash/",
			http.StatusBadRequest,
			"invalid camo url path: /two/withtrailingslash/, wanted 3 parts got 4\n"},
		{"/sYH2I8SV7JV_KpxO2zgGetvJbw/aHR0cDovL3Rlc3RpbmcuYmVwcmVzcy5jb20vc2lkX2dhbGxlcnlfb25lLzEwMTkvcHJldmlldy5qcGc",
			http.StatusForbidden,
			"invalid signature: mismatched length\n"},

		{"/sYH2aI8SV7JV_KpxO2zgGetvJbw/aHR0cDovL3Rlc3RpbmcuYmVwcmVzcy5jb20vc2lkX2dhbGxlcnlb25lLzEwMTkvcHJldmlldy5qcGc",
			http.StatusForbidden,
			"invalid signature: invalid mac\n"},
		{"/hzjem40Uq4-3xt4B8ZsGnQlCNM0/aHR0cDovLzE5Mi4xNjguMC4lMzEvbm90L3ZhbGlk", http.StatusForbidden, "Invalid downstream URL: parse http://192.168.0.%31/not/valid: invalid URL escape \"%31\"\n"},
	}

	tut := proxy.MustNew([]byte("test"), zerolog.New(ioutil.Discard))
	ts := httptest.NewTLSServer(tut)
	client := ts.Client()
	for _, test := range table {
		resp, err := client.Get(ts.URL + test.uri)
		checkers.OK(t, err)

		got, err := ioutil.ReadAll(resp.Body)
		checkers.OK(t, err)
		resp.Body.Close()
		checkers.Equals(t, resp.StatusCode, test.wantCode)
		checkers.Equals(t, string(got), test.wantMsg)
	}
}

func TestDefaultFilter(t *testing.T) {
	tut := proxy.MustNew([]byte("test"),
		zerolog.New(ioutil.Discard),
		func(p *proxy.Proxy) { p.Decoder = DummyDecoder{url: "http://10.1.10.1/someurl"} },
	)

	ts := httptest.NewTLSServer(tut)
	client := ts.Client()

	table := []struct {
		hostIP   string
		uri      string
		wantCode int
		wantMsg  string
	}{
		// TODO(ro) 2017-10-05 Add some 200's? Once we have a fake client.
		{"10.1.10.1", "/some/uri", 400, "invalid host: filtered host address: \"10.1.10.1\"\n"},
		{"127.0.0.1", "/local/host", 400, "invalid host: filtered host address: \"127.0.0.1\"\n"},
		{"ff02::2", "/ipv6/IPv6linklocalallnodes", 400, "invalid host: resolved to reserved address: \"ff02::2\"\n"},
		{"169.254.0.0", "/filtered/address", 400, "invalid host: filtered host address: \"169.254.0.0\"\n"},
		// mboned
		{"224.0.0.0", "/filtered/address", 400, "invalid host: filtered host address: \"224.0.0.0\"\n"},
		// ipv4 rfc1918
		{"10.0.0.33", "/filtered/address", 400, "invalid host: filtered host address: \"10.0.0.33\"\n"},
		{"172.16.0.2", "/filtered/address", 400, "invalid host: filtered host address: \"172.16.0.2\"\n"},
		{"192.168.0.6", "/filtered/address", 400, "invalid host: filtered host address: \"192.168.0.6\"\n"},
		// ipv6 loopback
		{"::1", "/filtered/address", 400, "invalid host: filtered host address: \"::1\"\n"},
		// ipv6 link local
		{"fe80::0", "/filtered/address", 400, "invalid host: filtered host address: \"fe80::\"\n"},
		// old ipv6 site local
		{"fec0::1", "/filtered/address", 400, "invalid host: filtered host address: \"fec0::1\"\n"},
		// ipv6 ULA
		{"fc00::7", "/filtered/address", 400, "invalid host: filtered host address: \"fc00::7\"\n"},
		{"::", "/i6/allzero", 400, "invalid host: resolved to reserved address: \"::\"\n"},
	}

	for _, test := range table {
		resolver := DummyResolver{ips: []net.IP{
			net.ParseIP(test.hostIP),
		}}
		tut.LookupIP = resolver.LookupIP
		tut.Decoder = DummyDecoder{url: "http://" + test.hostIP + test.uri}

		resp, err := client.Get(ts.URL + test.uri)
		checkers.OK(t, err)
		checkers.Equals(t, resp.StatusCode, test.wantCode)

		if test.wantCode != 200 {
			got, err := ioutil.ReadAll(resp.Body)
			checkers.OK(t, err)
			resp.Body.Close()
			checkers.Equals(t, resp.StatusCode, test.wantCode)
			checkers.Equals(t, string(got), test.wantMsg)
		}
	}
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

type DummyResolver struct {
	ips []net.IP
	err error
}

func (dr DummyResolver) LookupIP(s string) ([]net.IP, error) {
	if dr.err != nil {
		return nil, dr.err
	}
	return dr.ips, nil
}

func TestReverseProxyFlushInterval(t *testing.T) {
	const expected = "hi"
	// The backend server we proxy for.
	tsBE := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(expected))
	}))

	defer tsBE.Close()

	resolver := DummyResolver{ips: []net.IP{
		net.ParseIP("127.0.0.1"),
	}}

	tut := proxy.MustNew([]byte("test"),
		zerolog.New(ioutil.Discard),
		func(p *proxy.Proxy) { p.Decoder = DummyDecoder{url: tsBE.URL + "/lskdjf/lsdkjf"} },
		func(p *proxy.Proxy) { p.Filter = filter.MustNewCIDR([]string{}) },
		func(p *proxy.Proxy) { p.LookupIP = resolver.LookupIP },
		func(p *proxy.Proxy) { p.CheckUnicast = false },
		func(p *proxy.Proxy) { p.FlushInterval = time.Millisecond },
	)

	done := make(chan bool)
	proxy.OnExitFlushLoop = func() { done <- true }
	defer func() { proxy.OnExitFlushLoop = nil }()

	frontend := httptest.NewServer(tut)
	defer frontend.Close()

	req, _ := http.NewRequest("GET", frontend.URL+"/ldskjf/lsdkjf", nil)
	req.Close = true
	res, err := frontend.Client().Do(req)
	checkers.OK(t, err)
	defer res.Body.Close()
	bodyBytes, _ := ioutil.ReadAll(res.Body)
	checkers.Equals(t, string(bodyBytes), expected)

	select {
	case <-done:
		// OK
	case <-time.After(5 * time.Second):
		t.Error("maxLatencyWriter flushLoop() never exited")
	}
}

func TestResponseFound(t *testing.T) {
	resolver := DummyResolver{ips: []net.IP{
		net.ParseIP("127.0.0.1"),
	}}

	tsBE := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/unused")
		http.Error(w, "Found", http.StatusFound)
	}))

	defer tsBE.Close()

	tut := proxy.MustNew([]byte("test"),
		zerolog.New(ioutil.Discard),
		func(p *proxy.Proxy) { p.Decoder = DummyDecoder{url: tsBE.URL + "/lskdjf/lsdkjf"} },
		func(p *proxy.Proxy) { p.Filter = filter.MustNewCIDR([]string{}) },
		func(p *proxy.Proxy) { p.LookupIP = resolver.LookupIP },
		func(p *proxy.Proxy) { p.CheckUnicast = false },
		func(p *proxy.Proxy) {
			p.RedirFunc = func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}
		},
	)

	ts := httptest.NewTLSServer(tut)
	resp, err := ts.Client().Get(ts.URL + "/sldkjf/lsdkfj")
	checkers.OK(t, err)
	checkers.Equals(t, resp.StatusCode, http.StatusNotFound)

}

func TestResponseInternalError(t *testing.T) {
	resolver := DummyResolver{ips: []net.IP{
		net.ParseIP("127.0.0.1"),
	}}

	tsBE := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Found", http.StatusInternalServerError)
	}))

	defer tsBE.Close()

	tut := proxy.MustNew([]byte("test"),
		zerolog.New(ioutil.Discard),
		func(p *proxy.Proxy) { p.Decoder = DummyDecoder{url: tsBE.URL + "/lskdjf/lsdkjf"} },
		func(p *proxy.Proxy) { p.Filter = filter.MustNewCIDR([]string{}) },
		func(p *proxy.Proxy) { p.LookupIP = resolver.LookupIP },
		func(p *proxy.Proxy) { p.CheckUnicast = false },
		func(p *proxy.Proxy) {
			p.RedirFunc = func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}
		},
	)

	ts := httptest.NewTLSServer(tut)
	resp, err := ts.Client().Get(ts.URL + "/sldkjf/lsdkfj")
	checkers.OK(t, err)
	checkers.Equals(t, resp.StatusCode, http.StatusBadGateway)
}

func TestResponsePayloadTooLarge(t *testing.T) {
	resolver := DummyResolver{ips: []net.IP{
		net.ParseIP("127.0.0.1"),
	}}

	tsBE := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hi"))
	}))

	defer tsBE.Close()

	tut := proxy.MustNew([]byte("test"),
		zerolog.New(ioutil.Discard),
		func(p *proxy.Proxy) { p.MaxSize = 1 },
		func(p *proxy.Proxy) { p.Decoder = DummyDecoder{url: tsBE.URL + "/lskdjf/lsdkjf"} },
		func(p *proxy.Proxy) { p.Filter = filter.MustNewCIDR([]string{}) },
		func(p *proxy.Proxy) { p.LookupIP = resolver.LookupIP },
		func(p *proxy.Proxy) { p.CheckUnicast = false },
		func(p *proxy.Proxy) {
			p.RedirFunc = func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}
		},
	)

	ts := httptest.NewTLSServer(tut)
	resp, err := ts.Client().Get(ts.URL + "/sldkjf/lsdkfj")
	checkers.OK(t, err)
	checkers.Equals(t, resp.StatusCode, http.StatusRequestEntityTooLarge)
}
