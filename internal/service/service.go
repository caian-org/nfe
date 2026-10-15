// Package service ties the configuration, XML builders, signer and SOAP
// client together to execute the high-level emit / query / cancel use cases.
package service

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/caian-org/nfe/internal/abrasf"
	"github.com/caian-org/nfe/internal/config"
	"github.com/caian-org/nfe/internal/sigcorp"
	"github.com/caian-org/nfe/internal/soap"
	"github.com/caian-org/nfe/internal/xmlsig"
)

// soapCaller is the subset of *soap.Client used by Service. Defined here as
// an interface so service tests can swap in a stub.
type soapCaller interface {
	Call(ctx context.Context, action string, body []byte) ([]byte, error)
}

// restCaller is the subset of *sigcorp.Client used by the national flow.
type restCaller interface {
	Recepcionar(ctx context.Context, nNFSe string, dps []byte) (*sigcorp.Recepcao, []byte, error)
	Consultar(ctx context.Context, protocolo string) (*sigcorp.Consulta, []byte, error)
}

// Service orchestrates the NFS-e operations against ABRASF. A nil signer is
// allowed — operations that don't require signing still work, and emit /
// cancel will simply send unsigned XML (the WS may reject in that case, but
// dry-run inspection still works).
//
// The SOAP client is built lazily on first use so `emit --dry-run` works
// offline even when the WSDL endpoint is unreachable.
type Service struct {
	cfg      *config.Config
	soap     soapCaller
	soapErr  error
	soapOne  sync.Once
	rest     restCaller
	restErr  error
	restOne  sync.Once
	tlsCert  *tls.Certificate
	signer   *xmlsig.Signer
	progress ProgressFunc
	// now returns the date used for DataEmissao/Competencia. Injected so
	// tests can pin a deterministic day.
	now func() time.Time
}

// ProgressStatus describes the lifecycle state of one command step.
type ProgressStatus string

const (
	ProgressStarted ProgressStatus = "started"
	ProgressDone    ProgressStatus = "done"
	ProgressSkipped ProgressStatus = "skipped"
	ProgressFailed  ProgressStatus = "failed"
)

// ProgressEvent reports an operational step to the CLI. It intentionally
// carries only user-facing metadata so service logic stays UI-agnostic.
type ProgressEvent struct {
	Step   string
	Status ProgressStatus
	Detail string
}

// ProgressFunc receives service progress events.
type ProgressFunc func(ProgressEvent)

// Options bundles the wiring inputs of New.
type Options struct {
	Config   *config.Config
	SOAP     soapCaller     // when nil, New builds a default *soap.Client from Config
	REST     restCaller     // when nil, New builds a *sigcorp.Client from Config ([nacional])
	Signer   *xmlsig.Signer // when nil, New loads the A1 certificate from Config
	Now      func() time.Time
	Progress ProgressFunc
}

// New builds a Service. When SOAP is nil, the SOAP client is built lazily on
// first network call (so emit --dry-run can run offline). When Signer is nil
// and a certificate is configured, the certificate is loaded eagerly so the
// signer is available immediately for dry-run signing.
func New(opts Options) (*Service, error) {
	if opts.Config == nil {
		return nil, fmt.Errorf("service: Config é obrigatório")
	}
	s := &Service{cfg: opts.Config, soap: opts.SOAP, rest: opts.REST, signer: opts.Signer, now: opts.Now, progress: opts.Progress}
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
			InsecureSkipVerify: !s.cfg.IsProducao(),
			TLSCert:            s.tlsCert,
		}
		if endpoint := s.cfg.EndpointURL(); endpoint != "" {
			soapOpts.Endpoint = endpoint
			soapOpts.MethodNamespace = s.cfg.MethodNamespace()
			soapOpts.SoapActions = abrasfSOAPActions(s.cfg.SOAPActionPrefix())
		} else {
			soapOpts.WSDLURL = s.cfg.WSDLURL()
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

// ensureREST builds the SIGCORP client on first use (national mode only).
func (s *Service) ensureREST() error {
	s.restOne.Do(func() {
		if s.rest != nil {
			return
		}
		if !s.cfg.IsNacional() {
			s.restErr = errors.New("service: seção [nacional] ausente no config.toml")
			return
		}
		client, err := sigcorp.NewClient(sigcorp.Options{
			BaseURL: s.cfg.NacionalURL(),
			Usuario: s.cfg.NacionalUsuario(),
			Senha:   s.cfg.Autenticacao.Senha,
			CNPJ:    s.cfg.Prestador.CNPJ,
		})
		if err != nil {
			s.restErr = fmt.Errorf("service: construir cliente nacional: %w", err)
			return
		}
		s.rest = client
	})
	return s.restErr
}

// ErrNaoSuportadoNacional marks ABRASF-only operations invoked in national mode.
var ErrNaoSuportadoNacional = errors.New("operação indisponível no WS Padrão Nacional")

func abrasfSOAPActions(prefix string) map[string]string {
	ops := []string{
		"GerarNfse",
		"ConsultarNfseServicoPrestado",
		"ConsultarNfsePorRps",
		"CancelarNfse",
	}
	actions := make(map[string]string, len(ops))
	for _, op := range ops {
		actions[op] = prefix + op
	}
	return actions
}

// Config returns the underlying configuration.
func (s *Service) Config() *config.Config { return s.cfg }

func (s *Service) report(step string, status ProgressStatus, detail string) {
	if s.progress == nil {
		return
	}
	s.progress(ProgressEvent{Step: step, Status: status, Detail: detail})
}

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
