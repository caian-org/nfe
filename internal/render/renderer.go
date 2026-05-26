// Package render formats command results either as human-readable text or as
// machine-parsable JSON, depending on the global --json flag.
package render

import (
	"io"

	"github.com/caian-org/nfe/internal/config"
)

// StatusInfo summarises the active configuration for the `status` command.
type StatusInfo struct {
	Ambiente           string  `json:"ambiente"`
	WSDLURL            string  `json:"wsdl_url"`
	RazaoSocial        string  `json:"razao_social"`
	CNPJ               string  `json:"cnpj"`
	InscricaoMunicipal string  `json:"inscricao_municipal"`
	SerieRPS           string  `json:"serie_rps"`
	ProximoNumeroRPS   int     `json:"proximo_numero_rps"`
	AliquotaISS        float64 `json:"aliquota_iss"`
	CodigoMunicipio    string  `json:"codigo_municipio"`
	UsingCertificate   bool    `json:"using_certificate"`
	UsingBasicAuth     bool    `json:"using_basic_auth"`
}

// NewStatusInfo builds a StatusInfo from a loaded config.
func NewStatusInfo(cfg *config.Config) StatusInfo {
	hasUser := cfg.Autenticacao.Usuario != "" && cfg.Autenticacao.Senha != ""
	hasCert := cfg.Autenticacao.Certificado != nil &&
		cfg.Autenticacao.Certificado.Path != ""

	return StatusInfo{
		Ambiente:           cfg.Ambiente,
		WSDLURL:            cfg.WSDLURL(),
		RazaoSocial:        cfg.Prestador.RazaoSocial,
		CNPJ:               cfg.Prestador.CNPJ,
		InscricaoMunicipal: cfg.Prestador.InscricaoMunicipal,
		SerieRPS:           cfg.Configuracoes.SerieRPS,
		ProximoNumeroRPS:   cfg.Configuracoes.ProximoNumeroRPS,
		AliquotaISS:        cfg.Configuracoes.AliquotaISS,
		CodigoMunicipio:    cfg.Configuracoes.CodigoMunicipio,
		UsingCertificate:   hasCert,
		UsingBasicAuth:     hasUser,
	}
}

// Renderer writes command results to its underlying writer.
type Renderer interface {
	// Init reports a freshly scaffolded project.
	Init(path string, created []string) error
	// Env reports the new active environment after `nfe env`.
	Env(name string) error
	// Status renders the active configuration summary.
	Status(StatusInfo) error
	// Emit renders the outcome of an emission attempt.
	Emit(EmitInfo) error
	// Query renders the outcome of a query.
	Query(QueryInfo) error
	// Cancel renders the outcome of a cancellation attempt.
	Cancel(CancelInfo) error
}

// MensagemRetorno mirrors abrasf.MensagemRetorno but is decoupled from the
// XML wire types so renderers don't pull XML tags into JSON output.
type MensagemRetorno struct {
	Codigo   string `json:"codigo"`
	Mensagem string `json:"mensagem"`
	Correcao string `json:"correcao,omitempty"`
}

// EmitInfo is the user-facing view of an emission attempt.
type EmitInfo struct {
	DryRun     bool              `json:"dry_run"`
	Sucesso    bool              `json:"sucesso"`
	NumeroRPS  int               `json:"numero_rps"`
	NumeroNFSe string            `json:"numero_nfse,omitempty"`
	Mensagens  []MensagemRetorno `json:"mensagens,omitempty"`
	SignedXML  string            `json:"signed_xml,omitempty"`
	Response   string            `json:"response,omitempty"`
}

// QueryInfo is the user-facing view of a query.
type QueryInfo struct {
	Sucesso   bool              `json:"sucesso"`
	Mensagens []MensagemRetorno `json:"mensagens,omitempty"`
	NFSes     []QueriedNFSe     `json:"nfses,omitempty"`
	Response  string            `json:"response,omitempty"`
}

// QueriedNFSe is the parsed view of a single NFS-e returned by query.
type QueriedNFSe struct {
	Numero            string `json:"numero,omitempty"`
	CodigoVerificacao string `json:"codigo_verificacao,omitempty"`
	DataEmissao       string `json:"data_emissao,omitempty"`
	ValorServicos     string `json:"valor_servicos,omitempty"`
	RazaoSocialTomador string `json:"razao_social_tomador,omitempty"`
}

// CancelInfo is the user-facing view of a cancellation attempt.
type CancelInfo struct {
	Sucesso    bool              `json:"sucesso"`
	NumeroNFSe string            `json:"numero_nfse"`
	Codigo     int               `json:"codigo"`
	Mensagens  []MensagemRetorno `json:"mensagens,omitempty"`
	Response   string            `json:"response,omitempty"`
}

// New returns the appropriate renderer for the requested output mode.
// stdout receives successful command results; stderr is reserved for errors
// and log output.
func New(jsonOutput bool, stdout io.Writer) Renderer {
	if jsonOutput {
		return &jsonRenderer{w: stdout}
	}
	return &humanRenderer{w: stdout}
}
