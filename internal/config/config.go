// Package config loads and saves the NFS-e CLI configuration in TOML format,
// expanding ${VAR}/$VAR references against the process environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

const (
	EnvHomologacao = "homologacao"
	EnvProducao    = "producao"
)

// Config is the top-level configuration loaded from config.toml.
type Config struct {
	Ambiente      string        `toml:"ambiente"`
	SOAP          SOAP          `toml:"soap"`
	Prestador     Prestador     `toml:"prestador"`
	Autenticacao  Autenticacao  `toml:"autenticacao"`
	Configuracoes Configuracoes `toml:"configuracoes"`
	Nacional      *Nacional     `toml:"nacional,omitempty"`
	IBSCBS        *IBSCBS       `toml:"ibscbs,omitempty"`

	// configDir is the absolute directory of the loaded config file.
	// Used by CertificatePath to resolve relative cert paths. Unexported so
	// go-toml/v2 skips it on Marshal and Save() round-trips the original
	// relative path the user wrote on disk.
	configDir string
}

// SOAP holds the transport endpoints used for each environment. Making them
// configurable lets the same binary serve any ABRASF municipality.
type SOAP struct {
	WSDLHomologacao      string `toml:"wsdl_homologacao,omitempty"`
	WSDLProducao         string `toml:"wsdl_producao,omitempty"`
	EndpointHomologacao  string `toml:"endpoint_homologacao,omitempty"`
	EndpointProducao     string `toml:"endpoint_producao,omitempty"`
	NamespaceHomologacao string `toml:"namespace_homologacao,omitempty"`
	NamespaceProducao    string `toml:"namespace_producao,omitempty"`
	SOAPActionPrefix     string `toml:"soap_action_prefix,omitempty"`
}

type Prestador struct {
	CNPJ               string   `toml:"cnpj"`
	InscricaoMunicipal string   `toml:"inscricao_municipal"`
	RazaoSocial        string   `toml:"razao_social"`
	NomeFantasia       string   `toml:"nome_fantasia,omitempty"`
	Endereco           Endereco `toml:"endereco"`
	Contato            *Contato `toml:"contato,omitempty"`
}

type Endereco struct {
	Endereco        string `toml:"endereco"`
	Numero          string `toml:"numero"`
	Complemento     string `toml:"complemento,omitempty"`
	Bairro          string `toml:"bairro"`
	CodigoMunicipio string `toml:"codigo_municipio"`
	UF              string `toml:"uf"`
	CEP             string `toml:"cep"`
}

type Contato struct {
	Telefone string `toml:"telefone,omitempty"`
	Email    string `toml:"email,omitempty"`
}

type Autenticacao struct {
	Usuario     string       `toml:"usuario,omitempty"`
	Senha       string       `toml:"senha,omitempty"`
	Certificado *Certificado `toml:"certificado,omitempty"`
}

type Certificado struct {
	Path  string `toml:"path"`
	Senha string `toml:"senha"`
}

type Configuracoes struct {
	SerieRPS         string  `toml:"serie_rps"`
	ProximoNumeroRPS int     `toml:"proximo_numero_rps"`
	CodigoMunicipio  string  `toml:"codigo_municipio"`
	AliquotaISS      float64 `toml:"aliquota_iss"`
	ConfirmTimeout   string  `toml:"confirm_timeout,omitempty"`
	ConfirmInterval  string  `toml:"confirm_interval,omitempty"`
	// ProximoNumeroNFSe feeds the X-NFSe-nNFSe header of the SIGCORP national
	// webservice; the prestador numbers its own NFS-e there.
	ProximoNumeroNFSe int `toml:"proximo_numero_nfse,omitempty"`
}

// Nacional enables the SIGCORP "WS Padrão Nacional" transport: a REST API
// that receives the national DPS XML instead of the ABRASF SOAP envelope.
// When the section is absent the CLI keeps using [soap].
type Nacional struct {
	URLProducao     string `toml:"url_producao"`
	URLHomologacao  string `toml:"url_homologacao"`
	SerieDPS        string `toml:"serie_dps"`
	VersaoAplicacao string `toml:"versao_aplicacao,omitempty"`
	OpSimpNac       int    `toml:"op_simp_nac"`
	RegEspTrib      int    `toml:"reg_esp_trib"`
	AssinarDPS      *bool  `toml:"assinar_dps,omitempty"`
}

// IBSCBS holds the per-prestador defaults of the DPS IBSCBS group. Rates are
// percentages (0.1 means 0.1%). The group is only emitted when this section
// exists.
type IBSCBS struct {
	CST                           string   `toml:"cst"`
	CodigoClassificacaoTributaria string   `toml:"codigo_classificacao_tributaria"`
	CodigoIndicadorOperacao       string   `toml:"codigo_indicador_operacao"`
	AliquotaIBSUF                 *float64 `toml:"aliquota_ibs_uf"`
	AliquotaIBSMunicipal          *float64 `toml:"aliquota_ibs_municipal"`
	AliquotaCBS                   *float64 `toml:"aliquota_cbs"`
}

const (
	DefaultConfirmTimeout   = "2m"
	DefaultConfirmInterval  = "5s"
	DefaultSOAPActionPrefix = "nfs#"

	DefaultNacionalURLProducao    = "https://webservice.meumunicipio.online"
	DefaultNacionalURLHomologacao = "https://qa-webservice.meumunicipio.online"
	DefaultSerieDPS               = "1"
	DefaultVersaoAplicacao        = "nfe"

	OpSimpNacNaoOptante = 1
	OpSimpNacMEI        = 2
	OpSimpNacMEEPP      = 3
)

var (
	serieDPSRegex  = regexp.MustCompile(`^0{0,4}\d{1,5}$`)
	digits3Regex   = regexp.MustCompile(`^\d{3}$`)
	digits6Regex   = regexp.MustCompile(`^\d{6}$`)
	nNFSeMaxDigits = 13
	versaoAplicMax = 20
)

// IsNacional reports whether the SIGCORP national REST transport is active.
func (c *Config) IsNacional() bool {
	return c.Nacional != nil
}

// NacionalURL returns the REST base URL for the configured ambiente.
func (c *Config) NacionalURL() string {
	if c.Nacional == nil {
		return ""
	}
	if c.Ambiente == EnvProducao {
		return strings.TrimRight(c.Nacional.URLProducao, "/")
	}
	return strings.TrimRight(c.Nacional.URLHomologacao, "/")
}

// TpAmb maps ambiente to the DPS tpAmb code: 1 produção, 2 homologação.
func (c *Config) TpAmb() int {
	if c.Ambiente == EnvProducao {
		return 1
	}
	return 2
}

// NacionalUsuario returns the Basic Auth user for the national webservice:
// autenticacao.usuario when set, otherwise the prestador's inscrição municipal.
func (c *Config) NacionalUsuario() string {
	if c.Autenticacao.Usuario != "" {
		return c.Autenticacao.Usuario
	}
	return c.Prestador.InscricaoMunicipal
}

// AssinarDPS reports whether the DPS must be signed before posting. Defaults
// to true; it only takes effect when a certificate is configured.
func (c *Config) AssinarDPS() bool {
	if c.Nacional == nil {
		return false
	}
	if c.Nacional.AssinarDPS == nil {
		return true
	}
	return *c.Nacional.AssinarDPS
}

// VersaoAplicacao returns the verAplic value sent in the DPS.
func (c *Config) VersaoAplicacao() string {
	if c.Nacional == nil || c.Nacional.VersaoAplicacao == "" {
		return DefaultVersaoAplicacao
	}
	return c.Nacional.VersaoAplicacao
}

// WSDLURL returns the WSDL endpoint for the configured ambiente.
func (c *Config) WSDLURL() string {
	if c.Ambiente == EnvProducao {
		return c.SOAP.WSDLProducao
	}
	return c.SOAP.WSDLHomologacao
}

// EndpointURL returns the explicit SOAP endpoint for the configured ambiente,
// or an empty string when the client should discover it from the WSDL.
func (c *Config) EndpointURL() string {
	if c.Ambiente == EnvProducao {
		return c.SOAP.EndpointProducao
	}
	return c.SOAP.EndpointHomologacao
}

// MethodNamespace returns the explicit method namespace for the configured
// ambiente, or the endpoint URL as the ABRASF direct-endpoint fallback.
func (c *Config) MethodNamespace() string {
	if c.Ambiente == EnvProducao {
		if c.SOAP.NamespaceProducao != "" {
			return c.SOAP.NamespaceProducao
		}
		return c.SOAP.EndpointProducao
	}
	if c.SOAP.NamespaceHomologacao != "" {
		return c.SOAP.NamespaceHomologacao
	}
	return c.SOAP.EndpointHomologacao
}

// SOAPActionPrefix returns the configured SOAPAction prefix. ABRASF WSDLs
// usually declare values such as "nfs#GerarNfse".
func (c *Config) SOAPActionPrefix() string {
	if c.SOAP.SOAPActionPrefix == "" {
		return DefaultSOAPActionPrefix
	}
	return c.SOAP.SOAPActionPrefix
}

// IsProducao reports whether the configuration targets the production
// environment. Used to decide TLS verification strictness.
func (c *Config) IsProducao() bool {
	return c.Ambiente == EnvProducao
}

// Load reads the TOML file at path, expands environment variables, and
// validates the resulting Config.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("arquivo de configuração não encontrado: %s", path)
		}
		return nil, fmt.Errorf("leitura da configuração: %w", err)
	}

	expanded := expandEnv(string(raw))

	var cfg Config
	if err := toml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse da configuração: %w", err)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolver caminho da configuração: %w", err)
	}
	cfg.configDir = filepath.Dir(absPath)
	applyDefaults(&cfg)

	if errs := Validate(&cfg); len(errs) > 0 {
		return nil, fmt.Errorf("configuração inválida: %s", strings.Join(errs, "; "))
	}

	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.SOAP.SOAPActionPrefix == "" {
		cfg.SOAP.SOAPActionPrefix = DefaultSOAPActionPrefix
	}
	if cfg.Configuracoes.ConfirmTimeout == "" {
		cfg.Configuracoes.ConfirmTimeout = DefaultConfirmTimeout
	}
	if cfg.Configuracoes.ConfirmInterval == "" {
		cfg.Configuracoes.ConfirmInterval = DefaultConfirmInterval
	}
	if n := cfg.Nacional; n != nil {
		if n.URLProducao == "" {
			n.URLProducao = DefaultNacionalURLProducao
		}
		if n.URLHomologacao == "" {
			n.URLHomologacao = DefaultNacionalURLHomologacao
		}
		if n.SerieDPS == "" {
			n.SerieDPS = DefaultSerieDPS
		}
		if n.OpSimpNac == 0 {
			n.OpSimpNac = OpSimpNacNaoOptante
		}
		if cfg.Configuracoes.ProximoNumeroNFSe == 0 {
			cfg.Configuracoes.ProximoNumeroNFSe = 1
		}
	}
}

// ConfirmDurations returns the polling timeout and interval used after an
// asynchronous emission response.
func (c Configuracoes) ConfirmDurations() (timeout, interval time.Duration, err error) {
	timeout, err = parsePositiveDuration(c.ConfirmTimeout, "configuracoes.confirm_timeout")
	if err != nil {
		return 0, 0, err
	}
	interval, err = parsePositiveDuration(c.ConfirmInterval, "configuracoes.confirm_interval")
	if err != nil {
		return 0, 0, err
	}
	return timeout, interval, nil
}

func parsePositiveDuration(value, name string) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s deve ser uma duração válida (ex.: 30s, 2m)", name)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s deve ser maior que zero", name)
	}
	return d, nil
}

// CertificatePath returns the absolute path of the A1 certificate,
// resolving relative values against the directory of the loaded config
// file. Returns "" when no certificate is configured.
func (c *Config) CertificatePath() string {
	if c.Autenticacao.Certificado == nil {
		return ""
	}
	p := c.Autenticacao.Certificado.Path
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(c.configDir, p)
}

// Save serializes cfg to TOML and writes it atomically to path. The atomic
// write (tmp file + rename) is what makes the proximo_numero_rps counter
// update safe under concurrent reads.
func Save(path string, cfg *Config) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("serializar configuração: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config-*.toml.tmp")
	if err != nil {
		return fmt.Errorf("criar arquivo temporário: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("escrever arquivo temporário: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("fechar arquivo temporário: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("renomear: %w", err)
	}
	return nil
}

// Validate returns a slice of human-readable validation errors, mirroring
// validateConfigErrors in the TypeScript source.
func Validate(cfg *Config) []string {
	var errs []string

	if cfg.Ambiente != EnvHomologacao && cfg.Ambiente != EnvProducao {
		errs = append(errs, `ambiente deve ser "homologacao" ou "producao"`)
	}

	if cfg.Prestador.CNPJ == "" {
		errs = append(errs, "CNPJ do prestador é obrigatório")
	}
	if cfg.Prestador.InscricaoMunicipal == "" {
		errs = append(errs, "inscrição municipal do prestador é obrigatória")
	}

	hasUserAuth := cfg.Autenticacao.Senha != "" &&
		(cfg.Autenticacao.Usuario != "" || (cfg.Nacional != nil && cfg.Prestador.InscricaoMunicipal != ""))
	hasCertAuth := cfg.Autenticacao.Certificado != nil &&
		cfg.Autenticacao.Certificado.Path != "" &&
		cfg.Autenticacao.Certificado.Senha != ""

	if !hasUserAuth && !hasCertAuth {
		errs = append(errs, "autenticação é obrigatória: forneça usuário/senha ou certificado digital")
	}

	if cert := cfg.Autenticacao.Certificado; cert != nil {
		if cert.Path == "" {
			errs = append(errs, "caminho do certificado é obrigatório quando certificado está configurado")
		}
		if cert.Senha == "" {
			errs = append(errs, "senha do certificado é obrigatória quando certificado está configurado")
		}
		if cert.Path != "" {
			lower := strings.ToLower(cert.Path)
			if !strings.HasSuffix(lower, ".p12") && !strings.HasSuffix(lower, ".pfx") {
				errs = append(errs, "certificado deve ser um arquivo .p12 ou .pfx (formato A1)")
			}
		}
	}

	if cfg.Configuracoes.CodigoMunicipio == "" {
		errs = append(errs, "código do município é obrigatório")
	}
	if cfg.Nacional == nil {
		if cfg.SOAP.WSDLHomologacao == "" && cfg.SOAP.EndpointHomologacao == "" {
			errs = append(errs, "soap.wsdl_homologacao ou soap.endpoint_homologacao é obrigatório")
		}
		if cfg.SOAP.WSDLProducao == "" && cfg.SOAP.EndpointProducao == "" {
			errs = append(errs, "soap.wsdl_producao ou soap.endpoint_producao é obrigatório")
		}
	} else {
		errs = append(errs, validateNacional(cfg)...)
	}
	if cfg.IBSCBS != nil {
		errs = append(errs, validateIBSCBS(cfg.IBSCBS)...)
	}
	if _, _, err := cfg.Configuracoes.ConfirmDurations(); err != nil {
		errs = append(errs, err.Error())
	}

	return errs
}

func validateNacional(cfg *Config) []string {
	var errs []string
	n := cfg.Nacional
	if n.URLProducao == "" {
		errs = append(errs, "nacional.url_producao é obrigatório")
	}
	if n.URLHomologacao == "" {
		errs = append(errs, "nacional.url_homologacao é obrigatório")
	}
	if !serieDPSRegex.MatchString(n.SerieDPS) {
		errs = append(errs, "nacional.serie_dps deve ser numérica com até 5 dígitos (ex.: \"1\")")
	}
	if len(n.VersaoAplicacao) > versaoAplicMax {
		errs = append(errs, "nacional.versao_aplicacao deve ter até 20 caracteres")
	}
	if n.OpSimpNac < OpSimpNacNaoOptante || n.OpSimpNac > OpSimpNacMEEPP {
		errs = append(errs, "nacional.op_simp_nac deve ser 1 (não optante), 2 (MEI) ou 3 (ME/EPP)")
	}
	if n.RegEspTrib < 0 || n.RegEspTrib > 9 || (n.RegEspTrib > 6 && n.RegEspTrib < 9) {
		errs = append(errs, "nacional.reg_esp_trib deve ser 0-6 ou 9")
	}
	if cfg.NacionalUsuario() == "" || cfg.Autenticacao.Senha == "" {
		errs = append(errs, "modo nacional exige autenticacao.senha (senha do portal) e usuário (autenticacao.usuario ou prestador.inscricao_municipal)")
	}
	if cfg.Configuracoes.ProximoNumeroNFSe < 1 || len(strconv.Itoa(cfg.Configuracoes.ProximoNumeroNFSe)) > nNFSeMaxDigits {
		errs = append(errs, "configuracoes.proximo_numero_nfse deve ser um inteiro entre 1 e 13 dígitos")
	}
	return errs
}

func validateIBSCBS(i *IBSCBS) []string {
	var errs []string
	if !digits3Regex.MatchString(i.CST) {
		errs = append(errs, "ibscbs.cst deve conter exatamente 3 dígitos numéricos")
	}
	if !digits6Regex.MatchString(i.CodigoClassificacaoTributaria) {
		errs = append(errs, "ibscbs.codigo_classificacao_tributaria deve conter exatamente 6 dígitos numéricos")
	}
	if !digits6Regex.MatchString(i.CodigoIndicadorOperacao) {
		errs = append(errs, "ibscbs.codigo_indicador_operacao deve conter exatamente 6 dígitos numéricos")
	}
	for name, v := range map[string]*float64{
		"ibscbs.aliquota_ibs_uf":        i.AliquotaIBSUF,
		"ibscbs.aliquota_ibs_municipal": i.AliquotaIBSMunicipal,
		"ibscbs.aliquota_cbs":           i.AliquotaCBS,
	} {
		switch {
		case v == nil:
			errs = append(errs, name+" é obrigatória quando [ibscbs] está configurado")
		case *v < 0 || *v >= 100:
			errs = append(errs, name+" deve estar entre 0 e 100")
		}
	}
	sort.Strings(errs)
	return errs
}

// expandEnv expands ${VAR} and $VAR references in s against the process
// environment. References that point to unset variables are kept verbatim
// (matching the behaviour of the TypeScript yaml-loader).
func expandEnv(s string) string {
	return os.Expand(s, func(name string) string {
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		return "${" + name + "}"
	})
}
