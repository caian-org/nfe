// Package xmlsig signs ABRASF envelopes with an XMLDSig enveloped signature
// derived from an A1 PKCS#12 certificate.
package xmlsig

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

// LoadPFX reads an A1 (.p12/.pfx) certificate and returns the X.509 cert plus
// the RSA private key. ABRASF only supports RSA keys, so the type assertion is
// load-bearing.
func LoadPFX(path, password string) (*x509.Certificate, *rsa.PrivateKey, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("leitura do PFX: %w", err)
	}
	key, cert, _, err := pkcs12.DecodeChain(blob, password)
	if err != nil {
		return nil, nil, fmt.Errorf("decodificação do PFX: %w", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, nil, errors.New("chave privada do PFX não é RSA")
	}
	return cert, rsaKey, nil
}

// AsTLSCert builds a tls.Certificate from a parsed cert and key. The result
// plugs directly into tls.Config.Certificates for the SOAP client's mTLS.
func AsTLSCert(cert *x509.Certificate, key *rsa.PrivateKey) tls.Certificate {
	return tls.Certificate{
		Certificate: [][]byte{cert.Raw},
		PrivateKey:  key,
		Leaf:        cert,
	}
}
