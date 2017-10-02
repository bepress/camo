// Package encoding implements base64 decoding of signed urls for insecure
// asset proxying.
// Copyright (c) 2012-2016 Eli Janssen
// Copyright (c) 2017 Berkeley Electronic Press
// Copyright (c) 2017 Reed O'Brien reed@reedobrien.com
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.
package encoding

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
)

// Decoder is an interface that signs and encodes or verifies and decodes URLs
// for camo consumption.
type Decoder interface {
	Decode(string, string) (string, error)
}

// MustNewURLDecoder returns a new UrlDecoder or panics.
func MustNewURLDecoder(hmackey []byte) URLDecoder {
	if len(hmackey) == 0 {
		panic("empty hmac not allowed")
	}
	return URLDecoder{hmackey: hmackey}
}

// URLDecoder implements Decoder.
type URLDecoder struct {
	hmackey []byte
}

// Decode verifies the signature (digest) against the decoded url.  It ensures
// the url is properly verified via HMAC, and then decodes the url, returning
// the url (if valid) or an error.
func (ed URLDecoder) Decode(dig, url string) (string, error) {
	var (
		ub  []byte
		err error
	)

	ub, err = ed.b64DecodeURL(dig, url)

	if err != nil {
		return "", err
	}
	return string(ub), nil
}

func (ed URLDecoder) validateURL(macbytes []byte, urlbytes []byte) error {
	mac := hmac.New(sha1.New, ed.hmackey)
	mac.Write(urlbytes)
	macSum := mac.Sum(nil)

	// ensure lengths are equal. if not, return error.
	if len(macSum) != len(macbytes) {
		return fmt.Errorf("mismatched length")
	}

	if subtle.ConstantTimeCompare(macSum, macbytes) != 1 {
		return fmt.Errorf("invalid mac")
	}
	return nil
}

// b64DecodeURL ensures the url is properly verified via HMAC, and then
// decodes the url, returning the url (if valid) or an error.
func (ed URLDecoder) b64DecodeURL(encdig string, encURL string) ([]byte, error) {
	urlBytes, err := base64.RawURLEncoding.DecodeString(encURL)
	if err != nil {
		return nil, fmt.Errorf("bad url decode")
	}

	macBytes, err := base64.RawURLEncoding.DecodeString(encdig)
	if err != nil {
		return nil, fmt.Errorf("bad mac decode")
	}

	if err := ed.validateURL(macBytes, urlBytes); err != nil {
		return nil, fmt.Errorf("invalid signature: %s", err)
	}

	return urlBytes, nil
}
