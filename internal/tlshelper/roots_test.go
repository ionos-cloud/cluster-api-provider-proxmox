package tlshelper

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This block contains a 100Y valid self-signed ECDSA CA with key no
// longer existent.
const exampleRootCA = `-----BEGIN CERTIFICATE-----
MIIB6DCCAY+gAwIBAgIUIgQxz/rn8WLH3zyHICHh8Us552AwCgYIKoZIzj0EAwIw
STELMAkGA1UEBhMCREUxEDAOBgNVBAgMB0hhbWJ1cmcxEzARBgNVBAoMCkV4YW1w
bGUgQ0ExEzARBgNVBAMMCkV4YW1wbGUgQ0EwIBcNMjQxMTI4MTExMzM3WhgPMjEy
NDExMDQxMTEzMzdaMEkxCzAJBgNVBAYTAkRFMRAwDgYDVQQIDAdIYW1idXJnMRMw
EQYDVQQKDApFeGFtcGxlIENBMRMwEQYDVQQDDApFeGFtcGxlIENBMFkwEwYHKoZI
zj0CAQYIKoZIzj0DAQcDQgAEvQIYmiX4z2sv8sVqyC/eQ7e2JH1WQgIRuSWYVEOL
YKEQDHwMg1KKu3+McgHXLiZ0af0JDyd00em/g7k39RzNhqNTMFEwHQYDVR0OBBYE
FPDaKA2+m5nRhWESuw+61HdvmZiGMB8GA1UdIwQYMBaAFPDaKA2+m5nRhWESuw+6
1HdvmZiGMA8GA1UdEwEB/wQFMAMBAf8wCgYIKoZIzj0EAwIDRwAwRAIgQtj0ZhXK
RYrWwHMzIHXqggsU3NhnOUa/yeeQOExDVbMCIAFDdaz3v5jOmWm7LR5BwQwLRWhO
37ky2VQr/1GhU42q
-----END CERTIFICATE-----`

func TestRootPoolLoading(t *testing.T) {
	// Empty file
	roots, err := SystemRootsWithFile("")
	require.NoError(t, err)
	assert.Nil(t, roots)

	// Empty pemBlock
	roots, err = SystemRootsWithCert(nil)
	require.NoError(t, err)
	assert.Nil(t, roots)

	// CA from system plus PEM
	roots, err = SystemRootsWithCert([]byte(exampleRootCA))
	require.NoError(t, err)
	assert.NotNil(t, roots)

	// Error from broken file
	_, err = SystemRootsWithFile("/tmp/this/file/will/never/exist.notexist")
	require.Error(t, err)

	// Error from broken PEM
	_, err = SystemRootsWithCert([]byte("I'm certainly not a PEM block"))
	require.Error(t, err)
}
