package helpers_test

import (
	"os"
	"testing"

	"github.com/bepress/camo/checkers"
	"github.com/bepress/camo/helpers"
)

func TestGetHMAC(t *testing.T) {
	table := []struct {
		env    string
		secret string
		want   string
	}{
		{"", "secret", "secret"},
		{"env", "", "env"},
		{"env", "secret", "secret"},
		{"", "", ""},
	}
	for _, test := range table {
		os.Unsetenv(helpers.HMACEnvKey)
		if test.env != "" {
			os.Setenv(helpers.HMACEnvKey, test.env)
		}
		got := helpers.GetHMAC(test.secret)
		checkers.Equals(t, got, test.want)
	}
}
