package filter_test

import (
	"fmt"
	"testing"

	"github.com/bepress/camo/checkers"
	"github.com/bepress/camo/filter"
)

func TestNew(t *testing.T) {
	tut := filter.NewCIDR([]string{"10.0.0.0/8"})
	got, err := tut.Allowed("10.1.1.1")
	checkers.OK(t, err)
	checkers.Equals(t, got, false)
}

func TestIP4(t *testing.T) {
	var filtered = []string{
		// ipv4 loopback
		"127.0.0.0/8",
		// ipv4 link local
		"169.254.0.0/16",
		// ipv4 rfc1918
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}
	table := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1", false},
		{"127.0.1.1", false},
		{"8.8.8.8", true},
		{"169.254.55.123", false},
		{"10.0.1.10", false},
		{"192.255.1.2", true},
		{"172.16.1.1", false},
		{"173.16.0.1", true},
		{"192.168.1.1", false},
	}

	tut := filter.NewCIDR(filtered)
	for _, test := range table {
		got, err := tut.Allowed(test.addr)
		checkers.OK(t, err)
		checkers.Equals(t, got, test.want)
	}

}

func TestIP6(t *testing.T) {
	filtered := []string{
		// ipv6 loopback
		"::1/128",
		// ipv6 link local
		"fe80::/10",
		// old ipv6 site local
		"fec0::/10",
		// ipv6 ULA
		"fc00::/7",
		// ipv4 mapped onto ipv6
		"::ffff:0:0/96",
	}
	table := []struct {
		addr string
		want bool
	}{
		{"::1", false},
		{"fe80::1:1", false},
		{"2603:3024:100d:6200:bdc6:e7b5:21e2:7013", true},
		{"fec0::1:1", false},
		{"fc00::1:1", false},
		{"0:0:0:0:0:ffff:49fc:e3ab", false}, // 73.252.227.171 mapped to ipv6
	}

	tut := filter.NewCIDR(filtered)
	for _, test := range table {
		fmt.Println("testing:", test.addr)
		got, err := tut.Allowed(test.addr)
		checkers.OK(t, err)
		checkers.Equals(t, got, test.want)
	}

}
