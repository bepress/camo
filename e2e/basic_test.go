package camo_e2e

import (
	"testing"

	baloo "gopkg.in/h2non/baloo.v3"
)

// test stores the HTTP testing client preconfigured

var test = baloo.New("https://securedassets-staging.bepress.com")

// Testing http://httpbin.org/image/png
func TestHappyPathSimple(t *testing.T) {
	test.Get("/E4g4m1Ct9LmukxmzP3EKmBsGDHY/aHR0cDovL2h0dHBiaW4ub3JnL2ltYWdlL3BuZw").
		Expect(t).
		Status(200).
		HeaderEquals("Content-Length", "8090").
		HeaderEquals("Content-Security-Policy", "default-src https://securedassets.bepress.com").
		HeaderEquals("Strict-Transport-Security", "max-age=63072000; includeSubDomains").
		HeaderEquals("Via", "bepress/camo").
		Type("image/png").
		Done()
}
