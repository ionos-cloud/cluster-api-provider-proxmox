// Package tlshelper wraps loading and modifying the root-CA store for
// use in tls.Config
package tlshelper

import (
	"crypto/x509"
	"fmt"
	"os"
)

// SystemRootsWithFile reads the pemBlock for SystemRootsWithCert from
// the given file and then calls SystemRootsWithCert.
func SystemRootsWithFile(filepath string) (*x509.CertPool, error) {
	if len(filepath) == 0 {
		// No filename given, we fall back to default behavior: returning
		// nil and letting Go itself take care of everything
		return nil, nil
	}

	pemBlock, err := os.ReadFile(filepath) //#nosec:G304 // Intended to read the given file
	if err != nil {
		return nil, fmt.Errorf("loading certificate file: %w", err)
	}

	return SystemRootsWithCert(pemBlock)
}

// SystemRootsWithCert tries to load the system root store and to
// append the given certificate from a PEM encoded block into it. In
// case there is no pool an empty pool is used to add the certificate.
func SystemRootsWithCert(pemBlock []byte) (*x509.CertPool, error) {
	if len(pemBlock) == 0 {
		// Zero length / nil, we fall back to default behavior: returning
		// nil and letting Go itself take care of everything
		return nil, nil
	}

	rootCerts, err := x509.SystemCertPool()
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("loading system cert pool: %w", err)
	}

	if os.IsNotExist(err) {
		// The system pool is not available but that's fine as we are
		// supposed to add one own cert
		rootCerts = x509.NewCertPool()
	}

	if !rootCerts.AppendCertsFromPEM(pemBlock) {
		return nil, fmt.Errorf("certificate was not appended")
	}

	return rootCerts, nil
}
