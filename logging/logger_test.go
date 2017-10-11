package logging_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/bepress/camo/checkers"
	"github.com/bepress/camo/logging"
)

func TestQuietNewLogger(t *testing.T) {
	out := &bytes.Buffer{}
	l := logging.NewLogger("testapp", false, out)

	l.Debug().Msg("ignored")
	checkers.Equals(t, out.String(), "")

	l.Info().Msg("foo")
	checkers.Assert(t, strings.Contains(out.String(), "foo"), "expected string 'foo' missing")
}

func TestVerboseNewLogger(t *testing.T) {
	out := &bytes.Buffer{}
	l := logging.NewLogger("testapp", true, out)

	l.Debug().Msg("not ignored")
	checkers.Assert(t, strings.ContainsAny(out.String(), "not ignored"), "expected string 'not ignored' missing")

	l.Info().Msg("foo")
	checkers.Assert(t, strings.Contains(out.String(), "foo"), "expected string 'foo' missing")
}
