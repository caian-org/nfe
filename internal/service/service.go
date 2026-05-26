// Package service ties the configuration, XML builders, signer and SOAP
// client together to execute the high-level emit / query / cancel use cases.
package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/caian-org/nfe/internal/abrasf"
	"github.com/caian-org/nfe/internal/config"
	"github.com/caian-org/nfe/internal/soap"
	"github.com/caian-org/nfe/internal/xmlsig"
)

// soapCaller is the subset of *soap.Client used by Service. Defined here as
// an interface so service tests can swap in a stub.
type soapCaller interface {
	Call(ctx context.Context, action string, body []byte) ([]byte, error)
}

// Service orchestrates the NFS-e operations against ABRASF. A nil signer is
// allowed — operations that don't require signing still work, and emit /
// cancel will simply send unsigned XML (the WS may reject in that case, but
// dry-run inspection still works).
//
// The SOAP client is built lazily on first use so `emit --dry-run` works
// offline even when the WSDL endpoint is unreachable.
type Service struct {
	cfg     *config.Config
	soap    soapCaller
	soapErr error
	soapOne sync.Once
	tlsCert *tls.Certificate
	signer  *xmlsig.Signer
	// now returns the date used for DataEmissao/Competencia. Injected so
	// tests can pin a deterministic day.
	now func() time.Time
}

// Options bundles the wiring inputs of New.
type Options struct {
	Config *config.Config
	SOAP   soapCaller     // when nil, New builds a default *soap.Client from Config
	Signer *xmlsig.Signer // when nil, New loads the A1 certificate from Config
	Now    func() time.Time
}

// New builds a Service. When SOAP is nil, the SOAP client is built lazily on
// first network call (so emit --dry-run can run offline). When Signer is nil
// and a certificate is configured, the certificate is loaded eagerly so the
// signer is available immediately for dry-run signing.
func New(opts Options) (*Service, error) {
	if opts.Config == nil {
		return nil, fmt.Errorf("service: Config é obrigatório")
	}
	s := &Service{cfg: opts.Config, soap: opts.SOAP, signer: opts.Signer, now: opts.Now}
	if s.now == nil {
		s.now = time.Now
	}

	if opts.Signer == nil && opts.Config.Autenticacao.Certificado != nil &&
		opts.Config.Autenticacao.Certificado.Path != "" {
		cert, key, err := xmlsig.LoadPFX(
			opts.Config.CertificatePath(),
			opts.Config.Autenticacao.Certificado.Senha,
		)
		if err != nil {
			return nil, fmt.Errorf("service: carregar certificado: %w", err)
		}
		tlsCert := xmlsig.AsTLSCert(cert, key)
		s.tlsCert = &tlsCert
		signer, err := xmlsig.NewSigner(cert, key)
		if err != nil {
			return nil, fmt.Errorf("service: construir signer: %w", err)
		}
		s.signer = signer
	}

	return s, nil
}

// ensureSOAP builds the SOAP client on first use. Returns the same error on
// every subsequent call if the build failed (e.g. WSDL fetch errored).
func (s *Service) ensureSOAP() error {
	s.soapOne.Do(func() {
		if s.soap != nil {
			return
		}
		soapOpts := soap.Options{
			WSDLURL:            s.cfg.WSDLURL(),
			InsecureSkipVerify: !s.cfg.IsProducao(),
			TLSCert:            s.tlsCert,
		}
		if s.cfg.Autenticacao.Usuario != "" && s.cfg.Autenticacao.Senha != "" {
			soapOpts.BasicAuth = &soap.BasicAuth{
				Username: s.cfg.Autenticacao.Usuario,
				Password: s.cfg.Autenticacao.Senha,
			}
		}
		client, err := soap.NewClient(soapOpts)
		if err != nil {
			s.soapErr = fmt.Errorf("service: construir cliente SOAP: %w", err)
			return
		}
		s.soap = client
	})
	return s.soapErr
}

// Config returns the underlying configuration.
func (s *Service) Config() *config.Config { return s.cfg }

// MessagesError carries one or more MensagemRetorno entries surfaced by the
// WS. Callers can type-assert it to distinguish business-rule rejections from
// connection or input errors.
type MessagesError struct {
	Action    string
	Mensagens []abrasf.MensagemRetorno
	Raw       []byte
}

func (e *MessagesError) Error() string {
	if len(e.Mensagens) == 0 {
		return e.Action + ": sem mensagens de erro"
	}
	first := e.Mensagens[0]
	return fmt.Sprintf("%s: %s — %s", e.Action, first.Codigo, first.Mensagem)
}
