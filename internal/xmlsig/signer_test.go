package xmlsig_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	mathrand "math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkcs12 "software.sslmate.com/src/go-pkcs12"

	"github.com/caian-org/nfe/internal/abrasf"
	"github.com/caian-org/nfe/internal/config"
	"github.com/caian-org/nfe/internal/nota"
	"github.com/caian-org/nfe/internal/xmlsig"
)

// makeTestCert generates a small, fast RSA cert for tests.
func makeTestCert(t *testing.T) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(mathrand.Int63()),
		Subject:      pkix.Name{CommonName: "TEST-NFE-SIGNER"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert, key
}

// verifySignature performs a manual end-to-end XMLDSig verification of a
// signed Inf element. We re-implement validation here because goxmldsig's
// Validate detaches the element before canonicalizing, which loses inherited
// xmlns context — and ABRASF puts the xmlns on the root, not on Inf. The
// production WS uses Java/Santuario which does NOT detach, so the actual
// signature produced by our signer is correct for the WS; only the in-process
// goxmldsig verifier is incompatible. Reimplementing it here checks that the
// digest in the signed XML matches a fresh canonicalization of the same
// element (proving the signer is internally consistent) and that the RSA-SHA1
// signature is valid over the SignedInfo bytes.
// verifySignature mutates infEl in-place: it strips the Signature out of the
// target (the enveloped-signature transform) while leaving infEl attached to
// its parent, so the canonicalizer sees the same inherited xmlns the signer
// did. Pass a fresh parse if you need infEl back intact afterwards.
func verifySignature(t *testing.T, infEl *etree.Element, cert *x509.Certificate) {
	t.Helper()
	require.NotNil(t, infEl)
	sigEl := infEl.SelectElement("Signature")
	require.NotNil(t, sigEl, "Inf must contain a Signature element")
	verifySignatureWithElement(t, infEl, sigEl, cert, true)
}

func verifySignatureWithElement(t *testing.T, infEl, sigEl *etree.Element, cert *x509.Certificate, removeFromInf bool) {
	t.Helper()
	require.NotNil(t, infEl)
	require.NotNil(t, sigEl)
	signedInfo := sigEl.SelectElement("SignedInfo")
	require.NotNil(t, signedInfo)

	digestValueEl := signedInfo.FindElement(".//DigestValue")
	require.NotNil(t, digestValueEl)
	expectedDigest, err := base64.StdEncoding.DecodeString(digestValueEl.Text())
	require.NoError(t, err)

	// Canonicalize the SignedInfo while it still has its full namespace
	// context from the doc.
	c14n := dsig.MakeC14N10RecCanonicalizer()
	canonicalSI, err := c14n.Canonicalize(signedInfo)
	require.NoError(t, err)

	if removeFromInf {
		// Strip Signature in place — infEl stays attached so canonicalization
		// preserves the inherited xmlns the signer used.
		infEl.RemoveChild(sigEl)
	}

	canonical, err := c14n.Canonicalize(infEl)
	require.NoError(t, err)
	gotDigest := sha1.Sum(canonical)
	assert.Equal(t, expectedDigest, gotDigest[:],
		"digest in signed XML must match a fresh canonicalization of Inf with its parent context")

	// Verify the RSA-SHA1 signature is valid over the canonicalized SignedInfo.
	sigValueEl := sigEl.SelectElement("SignatureValue")
	require.NotNil(t, sigValueEl)
	sigBytes, err := base64.StdEncoding.DecodeString(sigValueEl.Text())
	require.NoError(t, err)
	hash := sha1.Sum(canonicalSI)
	require.NoError(t, rsa.VerifyPKCS1v15(cert.PublicKey.(*rsa.PublicKey), crypto.SHA1, hash[:], sigBytes))
}

func TestSignGerarNfse(t *testing.T) {
	cert, key := makeTestCert(t)
	signer, err := xmlsig.NewSigner(cert, key)
	require.NoError(t, err)

	cfg := config.Default()
	cfg.Configuracoes.ProximoNumeroRPS = 7
	in := &nota.Input{
		Tomador: nota.Tomador{
			CNPJ:        "44555666000170",
			RazaoSocial: "TOMADOR EXEMPLO",
			Endereco: nota.Endereco{
				Endereco:        "RUA EXEMPLO",
				Numero:          "1",
				Bairro:          "CENTRO",
				CodigoMunicipio: "7654321",
				UF:              "SP",
				CEP:             "01000000",
			},
		},
		Servico: nota.Servico{
			Discriminacao:    "TEST",
			ValorServicos:    100.0,
			ItemListaServico: "0101",
			Aliquota:         5.0,
		},
	}

	inf := abrasf.BuildRPS(cfg, in, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	unsigned, err := abrasf.BuildGerarNfse(inf)
	require.NoError(t, err)

	signed, err := signer.Sign(unsigned, "InfDeclaracaoPrestacaoServico", "rps7")
	require.NoError(t, err)
	require.NotEmpty(t, signed)

	doc := etree.NewDocument()
	require.NoError(t, doc.ReadFromBytes(signed))

	infEl := doc.FindElement("//InfDeclaracaoPrestacaoServico[@Id='rps7']")
	require.NotNil(t, infEl, "InfDeclaracaoPrestacaoServico must remain in signed document")
	sigEl := infEl.SelectElement("Signature")
	require.NotNil(t, sigEl, "Signature must be a child of Inf")
	children := infEl.ChildElements()
	assert.Equal(t, children[len(children)-1], sigEl, "Signature must be the LAST child of Inf")

	// Structural assertions: algorithms, certificate, reference URI.
	signedInfo := sigEl.SelectElement("SignedInfo")
	require.NotNil(t, signedInfo)
	canonMethod := signedInfo.SelectElement("CanonicalizationMethod")
	require.NotNil(t, canonMethod)
	assert.Equal(t, "http://www.w3.org/TR/2001/REC-xml-c14n-20010315",
		canonMethod.SelectAttrValue("Algorithm", ""))
	sigMethod := signedInfo.SelectElement("SignatureMethod")
	require.NotNil(t, sigMethod)
	assert.Equal(t, "http://www.w3.org/2000/09/xmldsig#rsa-sha1",
		sigMethod.SelectAttrValue("Algorithm", ""))
	ref := signedInfo.SelectElement("Reference")
	require.NotNil(t, ref)
	assert.Equal(t, "#rps7", ref.SelectAttrValue("URI", ""))
	digestMethod := ref.SelectElement("DigestMethod")
	require.NotNil(t, digestMethod)
	assert.Equal(t, "http://www.w3.org/2000/09/xmldsig#sha1",
		digestMethod.SelectAttrValue("Algorithm", ""))
	x509Cert := sigEl.FindElement(".//X509Certificate")
	require.NotNil(t, x509Cert)
	assert.NotEmpty(t, x509Cert.Text(), "X509Certificate must contain the cert DER")

	// Verify the signature numerically.
	verifySignature(t, infEl, cert)
}

func TestSignSiblingGerarNfse(t *testing.T) {
	cert, key := makeTestCert(t)
	signer, err := xmlsig.NewSigner(cert, key)
	require.NoError(t, err)

	cfg := config.Default()
	cfg.Configuracoes.ProximoNumeroRPS = 7
	inf := abrasf.BuildRPS(cfg, &nota.Input{
		Tomador: nota.Tomador{
			CNPJ:        "44555666000170",
			RazaoSocial: "TOMADOR EXEMPLO",
			Endereco: nota.Endereco{
				Endereco:        "RUA EXEMPLO",
				Numero:          "1",
				Bairro:          "CENTRO",
				CodigoMunicipio: "7654321",
				UF:              "SP",
				CEP:             "01000000",
			},
		},
		Servico: nota.Servico{
			Discriminacao:    "TEST",
			ValorServicos:    100.0,
			ItemListaServico: "0101",
			Aliquota:         5.0,
		},
	}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	unsigned, err := abrasf.BuildGerarNfse(inf)
	require.NoError(t, err)

	signed, err := signer.SignSibling(unsigned, "InfDeclaracaoPrestacaoServico", "rps7")
	require.NoError(t, err)

	doc := etree.NewDocument()
	require.NoError(t, doc.ReadFromBytes(signed))

	infEl := doc.FindElement("//InfDeclaracaoPrestacaoServico[@Id='rps7']")
	require.NotNil(t, infEl)
	assert.Nil(t, infEl.SelectElement("Signature"), "Signature must not be a child of Inf")

	rpsEl := infEl.Parent()
	require.NotNil(t, rpsEl)
	sigEl := rpsEl.SelectElement("Signature")
	require.NotNil(t, sigEl, "Signature must be a child of the Rps container")
	children := rpsEl.ChildElements()
	require.Len(t, children, 2)
	assert.Equal(t, infEl, children[0])
	assert.Equal(t, sigEl, children[1])

	ref := sigEl.FindElement(".//Reference")
	require.NotNil(t, ref)
	assert.Equal(t, "#rps7", ref.SelectAttrValue("URI", ""))
	verifySignatureWithElement(t, infEl, sigEl, cert, false)
}

func TestSignCancelarNfse(t *testing.T) {
	cert, key := makeTestCert(t)
	signer, err := xmlsig.NewSigner(cert, key)
	require.NoError(t, err)

	unsigned, err := abrasf.BuildCancelarNfse(abrasf.CancelInput{
		NumeroNfse:         "100",
		CNPJ:               "11222333000181",
		InscricaoMunicipal: "123456",
		CodigoMunicipio:    "1234567",
		Codigo:             abrasf.CancelErroEmissao,
	})
	require.NoError(t, err)

	signed, err := signer.Sign(unsigned, "InfPedidoCancelamento", "")
	require.NoError(t, err)

	doc := etree.NewDocument()
	require.NoError(t, doc.ReadFromBytes(signed))

	infEl := doc.FindElement("//InfPedidoCancelamento")
	require.NotNil(t, infEl)
	sigEl := infEl.SelectElement("Signature")
	require.NotNil(t, sigEl)
	last := infEl.ChildElements()
	assert.Equal(t, last[len(last)-1], sigEl, "Signature must be the LAST child of Inf")

	ref := sigEl.FindElement(".//Reference")
	require.NotNil(t, ref)
	assert.Equal(t, "", ref.SelectAttrValue("URI", "missing"),
		"Reference URI must be empty when target has no Id")

	verifySignature(t, infEl, cert)
}

func TestSignReturnsErrorWhenTargetMissing(t *testing.T) {
	cert, key := makeTestCert(t)
	signer, err := xmlsig.NewSigner(cert, key)
	require.NoError(t, err)

	_, err = signer.Sign([]byte(`<a><b/></a>`), "NotThere", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "não encontrado")

	_, err = signer.Sign([]byte(`<a><b Id="x"/></a>`), "b", "y")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "não encontrado")
}

func TestSignReturnsErrorOnMalformedXML(t *testing.T) {
	cert, key := makeTestCert(t)
	signer, err := xmlsig.NewSigner(cert, key)
	require.NoError(t, err)
	_, err = signer.Sign([]byte(`<a><b></a>`), "b", "")
	require.Error(t, err)
}

func TestSignReturnsErrorWhenTargetHasNoParent(t *testing.T) {
	cert, key := makeTestCert(t)
	signer, err := xmlsig.NewSigner(cert, key)
	require.NoError(t, err)
	// A document whose root IS the target — no parent to receive the signature.
	_, err = signer.Sign([]byte(`<InfPedidoCancelamento><x/></InfPedidoCancelamento>`),
		"InfPedidoCancelamento", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "não tem pai")
}

func TestNewSignerRejectsNilInputs(t *testing.T) {
	_, err := xmlsig.NewSigner(nil, nil)
	require.Error(t, err)
}

func TestLoadPFXRoundTrip(t *testing.T) {
	cert, key := makeTestCert(t)
	blob, err := pkcs12.Modern.WithRand(rand.Reader).Encode(key, cert, nil, "hunter2")
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "test.pfx")
	require.NoError(t, os.WriteFile(path, blob, 0o600))

	gotCert, gotKey, err := xmlsig.LoadPFX(path, "hunter2")
	require.NoError(t, err)
	assert.Equal(t, cert.SerialNumber, gotCert.SerialNumber)
	assert.NotNil(t, gotKey)

	tlsCert := xmlsig.AsTLSCert(gotCert, gotKey)
	require.NotEmpty(t, tlsCert.Certificate)
	require.NotNil(t, tlsCert.PrivateKey)
}

func TestLoadPFXMissing(t *testing.T) {
	_, _, err := xmlsig.LoadPFX("/no/such/path.pfx", "x")
	require.Error(t, err)
}

func TestLoadPFXWrongPassword(t *testing.T) {
	cert, key := makeTestCert(t)
	blob, err := pkcs12.Modern.WithRand(rand.Reader).Encode(key, cert, nil, "hunter2")
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "test.pfx")
	require.NoError(t, os.WriteFile(path, blob, 0o600))
	_, _, err = xmlsig.LoadPFX(path, "wrong-pwd")
	require.Error(t, err)
}
