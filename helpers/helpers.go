package helpers

import "os"

// HMACEnvKey is the string key for storing an HMAC secret in an environment
// variable.
const HMACEnvKey = "CAMO_HMAC_SECRET"

// GetHMAC get the secret from the passed in value or the environment if the
// paramter provided is empty.
func GetHMAC(s string) string {
	var hmac string
	if s != "" {
		hmac = s
		return hmac
	}
	hmac = os.Getenv(HMACEnvKey)
	return hmac
}
