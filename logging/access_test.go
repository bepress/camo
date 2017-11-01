package logging_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bepress/camo/checkers"
	"github.com/bepress/camo/logging"
	"github.com/bepress/camo/rxid"
	"github.com/rs/zerolog"
)

func TestAccessLogger(t *testing.T) {
	table := []struct {
		desc         string
		host         string
		method       string
		uri          string
		protocol     string
		status       int
		response     string
		referrer     string
		userAgent    string
		forwardedFor string
		rxid         string
	}{
		{"test 1", "example.com", "GET", "/blah", "HTTP/1.1", 200, "foo", "http://locahost/bar", "Go-http-client/1.1", "10.1.1.1", "rxid"},
		{"test POST path only", "example.com", "POST", "/blah", "HTTP/1.1", 200, "foo", "http://locahost/bar", "Go-http-client/1.1", "10.1.1.1, 192.168.1.1", "anid"},
		{"test GET path with query", "www.example.com", "GET", "/blah?a=b&a=c", "HTTP/1.1", 200, "foo", "http://locahost/bar", "Go-http-client/1.1", "10.1.1.1, 172.16.0.1, 192.168.4.5", "someid"},
		{"test GET path with query", "www.example.com", "POST", "/blah?a=b&a=c", "HTTP/1.1", 200, "foo", "http://locahost/bar", "Go-http-client/1.1", "", "lookandid"},
	}

	out := &bytes.Buffer{}

	zerolog.TimeFieldFormat = logging.TimeFormat
	logger := zerolog.New(out).With().
		Timestamp().
		Str("app", "testapp").
		Str("app_host", "testhost").
		Logger()

	client := &http.Client{}

	for _, test := range table {
		out.Reset()
		ts := httptest.NewServer(rxid.Handler(logging.NewAccessLogger(http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(test.response))
			}), logger)))
		defer ts.Close()

		req, err := http.NewRequest(test.method, ts.URL+test.uri, nil)
		checkers.OK(t, err)
		req.Header.Add("Referer", test.referrer)
		req.Header.Add("X-Forwarded-For", test.forwardedFor)

		req.Header.Add("X-Request-ID", test.rxid)

		req.Host = test.host

		res, err := client.Do(req)
		checkers.OK(t, err)

		b, err := ioutil.ReadAll(res.Body)
		checkers.OK(t, err)
		defer res.Body.Close()
		checkers.Equals(t, string(b), test.response)

		got := &proxyLogRecord{}
		err = json.Unmarshal(out.Bytes(), got)
		checkers.OK(t, err)

		checkers.Equals(t, got.App, "testapp")
		checkers.Equals(t, got.AppHost, "testhost")
		checkers.Equals(t, got.UserAgent, test.userAgent)
		checkers.Equals(t, got.Referrer, test.referrer)
		checkers.Equals(t, got.Domain, test.host)
		checkers.Equals(t, got.ClientIP, "127.0.0.1")
		checkers.Equals(t, got.ReponseBytes, len(test.response))
		checkers.Equals(t, got.XForwardedFor, strings.Split(test.forwardedFor, ", "))
		checkers.Equals(t, got.RequestID, test.rxid)
	}

}

type proxyLogRecord struct {
	RequestID     string   `json:"request_id"`
	Time          string   `json:"time"`
	Level         string   `json:"level"`
	App           string   `json:"app"`
	AppHost       string   `json:"app_host"`
	ClientIP      string   `json:"client_ip"`
	Duration      float64  `json:"duration"`
	Domain        string   `json:"domain"`
	Method        string   `json:"method"`
	URI           string   `json:"uri"`
	Protocol      string   `json:"protocol"`
	Status        int      `json:"status"`
	ReponseBytes  int      `json:"reponse_bytes"`
	Referrer      string   `json:"referrer"`
	UserAgent     string   `json:"user_agent"`
	XForwardedFor []string `json:"x_forwarded_for"`
}
