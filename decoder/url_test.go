// Copyright (c) 2012-2016 Eli Janssen
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package decoder_test

import (
	"strings"
	"testing"

	"github.com/bepress/camo/checkers"
	"github.com/bepress/camo/decoder"
)

func TestMustNewURLDecoderPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("MustNewURLDecoder failed to panic")
		}
	}()

	_ = decoder.MustNew(nil)
}

func TestDecodeURLs(t *testing.T) {
	table := []struct {
		desc    string
		errStr  string
		in      string
		want    string
		wantErr bool
		hmackey string
	}{
		{"test success host only url", "", "/I2s_jHIbZkwmHHX8wb8hmdxDM1g/aHR0cDovL2JlcHJlc3MuY29t", "http://bepress.com", false, "test"},
		{"test failing host url", "invalid signature: invalid mac", "/I2s_jHIbZkwmHHX8wb8hmdxDM1g/aH0cDovL2JlcHJlc3MuY29t", "", true, "test"},
		{"test failing host digest", "invalid signature: mismatched length", "/I2s_jHIbZkwmHHX8wb8hmdxDM1/aHR0cDovL2JlcHJlc3MuY29t", "", true, "test"},
		{"test failing hmackey ", "invalid signature: invalid mac", "/I2s_jHIbZkwmHHX8wb8hmdxDM1g/aHR0cDovL2JlcHJlc3MuY29t", "", true, "wrong"},
		{"test failing url encoding", "bad url decode", "/I2s_jHIbZkwmHHX8wb8hmdxDM1g/aHR0/cDovL2JlcHJlc3MuY29t", "", true, "wrong"},
		{"test failing url encoding", "bad mac decode", "/I2s_jHI=bZkwmHHX8wb8hmdxDM1g/aHR0cDovL2JlcHJlc3MuY29t", "", true, "wrong"},
	}

	for _, test := range table {
		tut := decoder.MustNew([]byte(test.hmackey))
		dig, url := serverSplitURL(test.in)
		got, err := tut.Decode(dig, url)
		if test.wantErr {
			checkers.Equals(t, err.Error(), test.errStr)
			continue
		}
		checkers.OK(t, err)
		checkers.Equals(t, got, test.want)
	}
}

func serverSplitURL(s string) (string, string) {
	parts := strings.SplitN(s, "/", 3)
	return parts[1], parts[2]
}

func BenchmarkDecode(b *testing.B) {
	tut := decoder.MustNew([]byte("test"))
	fut := tut.Decode
	for i := 0; i < b.N; i++ {
		fut("D23vHLFHsOhPOcvdxeoQyAJTpvM", "aHR0cDovL2dvbGFuZy5vcmcvZG9jL2dvcGhlci9mcm9udHBhZ2UucG5n")
	}
}
