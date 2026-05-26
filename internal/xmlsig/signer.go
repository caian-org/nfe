package xmlsig

import (
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
)

// Signer wraps a goxmldsig SigningContext configured for ABRASF (RSA-SHA1,
// inclusive C14N, no ds: prefix on the resulting Signature element).
type Signer struct {
	ctx *dsig.SigningContext
}

// memKeyStore satisfies dsig.X509KeyStore using an in-memory cert/key pair.
type memKeyStore struct {
	cert *x509.Certificate
	key  *rsa.PrivateKey
}

func (m *memKeyStore) GetKeyPair() (privateKey *rsa.PrivateKey, cert []byte, err error) {
	return m.key, m.cert.Raw, nil
}

// NewSigner configures a Signer for ABRASF's required algorithm suite.
func NewSigner(cert *x509.Certificate, key *rsa.PrivateKey) (*Signer, error) {
	if cert == nil || key == nil {
		return nil, errors.New("xmlsig: cert e key são obrigatórios")
	}
	ctx := &dsig.SigningContext{
		Hash:          crypto.SHA1,
		KeyStore:      &memKeyStore{cert: cert, key: key},
		IdAttribute:   "Id",
		Prefix:        "", // ABRASF expects <Signature xmlns="..."> with no namespace prefix.
		Canonicalizer: dsig.MakeC14N10RecCanonicalizer(),
	}
	return &Signer{ctx: ctx}, nil
}

// Sign produces an enveloped signature over the element matching targetName
// and (optionally) targetID. The Signature is appended as the last child of
// that element's parent, matching the original xml-crypto implementation.
//
// The returned bytes are the new full document.
func (s *Signer) Sign(xmlBytes []byte, targetName, targetID string) ([]byte, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(xmlBytes); err != nil {
		return nil, fmt.Errorf("xmlsig: parse: %w", err)
	}

	root := doc.Root()
	if root == nil {
		return nil, errors.New("xmlsig: documento vazio")
	}

	target := findElement(root, targetName, targetID)
	if target == nil {
		if targetID != "" {
			return nil, fmt.Errorf("xmlsig: elemento %s[@Id=%q] não encontrado", targetName, targetID)
		}
		return nil, fmt.Errorf("xmlsig: elemento %s não encontrado", targetName)
	}

	parent := target.Parent()
	// etree.Document embeds Element, so the document root's Parent is the
	// Document's embedded Element (with an empty Tag). Treat both nil and
	// "the Document itself" as no real parent.
	if parent == nil || parent.Tag == "" {
		return nil, errors.New("xmlsig: elemento alvo não tem pai para receber a assinatura")
	}

	// Sign the target directly. goxmldsig.SignEnveloped returns a copy of the
	// target with <Signature> appended as its last child. The Reference URI is
	// "#<Id>" when the target carries an Id, else "". The enveloped-signature
	// transform ensures the digest covers the target *without* the Signature.
	signedTarget, err := s.ctx.SignEnveloped(target)
	if err != nil {
		return nil, fmt.Errorf("xmlsig: assinatura do envelope: %w", err)
	}

	// Swap the original target for the signed copy at the same position so the
	// surrounding document structure is preserved.
	idx := target.Index()
	parent.RemoveChild(target)
	parent.InsertChildAt(idx, signedTarget)

	return doc.WriteToBytes()
}

// findElement performs a depth-first search rooted at el for an element whose
// local tag matches name and (when idAttr is non-empty) whose Id attribute
// matches. Returns the first match.
func findElement(el *etree.Element, name, idAttr string) *etree.Element {
	if el.Tag == name {
		if idAttr == "" || el.SelectAttrValue("Id", "") == idAttr {
			return el
		}
	}
	for _, child := range el.ChildElements() {
		if found := findElement(child, name, idAttr); found != nil {
			return found
		}
	}
	return nil
}
