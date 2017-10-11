package logging_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bepress/camo/checkers"
	"github.com/bepress/camo/logging"
	"github.com/rs/zerolog"
)

func TestAccessLogger(t *testing.T) {
	table := []struct {
		desc      string
		host      string
		method    string
		uri       string
		protocol  string
		status    int
		response  string
		referrer  string
		userAgent string
	}{
		{"test 1", "example.com", "GET", "/blah", "HTTP/1.1", 200, "foo", "http://locahost/bar", "Go-http-client/1.1"},
		{"test POST path only", "example.com", "POST", "/blah", "HTTP/1.1", 200, "foo", "http://locahost/bar", "Go-http-client/1.1"},
		{"test GET path with query", "www.example.com", "GET", "/blah?a=b&a=c", "HTTP/1.1", 200, "foo", "http://locahost/bar", "Go-http-client/1.1"},
		{"test GET path with query", "www.example.com", "POST", "/blah?a=b&a=c", "HTTP/1.1", 200, "foo", "http://locahost/bar", "Go-http-client/1.1"},
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
		ts := httptest.NewServer(logging.NewAccessLogger(http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintln(w, string(test.response))
			}), logger))
		defer ts.Close()

		req, err := http.NewRequest(test.method, ts.URL+test.uri, nil)
		checkers.OK(t, err)
		req.Header.Add("Referer", test.referrer)
		req.Host = test.host

		res, err := client.Do(req)
		checkers.OK(t, err)

		b, err := ioutil.ReadAll(res.Body)
		checkers.OK(t, err)
		defer res.Body.Close()
		checkers.Equals(t, string(b), test.response+"\n")

		got := &proxyLogRecord{}
		err = json.Unmarshal(out.Bytes(), got)
		checkers.OK(t, err)

		checkers.Equals(t, got.App, "testapp")
		checkers.Equals(t, got.AppHost, "testhost")
		checkers.Equals(t, got.UserAgent, test.userAgent)
		checkers.Equals(t, got.Referrer, test.referrer)
		checkers.Equals(t, got.Domain, test.host)
		checkers.Equals(t, got.ClientIP, "127.0.0.1")
		checkers.Equals(t, got.ReponseBytes, len(test.response)+1)
	}

}

type proxyLogRecord struct {
	Time         string  `json:"time"`
	Level        string  `json:"level"`
	App          string  `json:"app"`
	AppHost      string  `json:"app_host"`
	ClientIP     string  `json:"client_ip"`
	Duration     float64 `json:"duration"`
	Domain       string  `json:"domain"`
	Method       string  `json:"method"`
	URI          string  `json:"uri"`
	Protocol     string  `json:"protocol"`
	Status       int     `json:"status"`
	ReponseBytes int     `json:"reponse_bytes"`
	Referrer     string  `json:"referrer"`
	UserAgent    string  `json:"user_agent"`
}
