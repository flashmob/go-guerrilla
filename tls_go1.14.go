// +build !go1.14

package guerrilla

import "crypto/tls"

func init() {

	TLSProtocols["ssl3.0"] = tls.VersionSSL30 // deprecated since GO 1.13, removed 1.14

	// Include to prevent downgrade attacks (SSLv3 only, deprecated in Go 1.13)
	TLSCiphers["TLS_FALLBACK_SCSV"] = tls.TLS_FALLBACK_SCSV
}
