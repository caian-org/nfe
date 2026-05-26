// Package config loads and saves the NFS-e CLI configuration in TOML format,
// expanding ${VAR}/$VAR references against the process environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	// configDir is the absolute directory of the loaded config file.
	// Used by CertificatePath to resolve relative cert paths. Unexported so
	// go-toml/v2 skips it on Marshal and Save() round-trips the original
	// relative path the user wrote on disk.
	configDir string
}

// SOAP holds the WSDL endpoints used for each environment. Making them
// configurable lets the same binary serve any ABRASF municipality.
type SOAP struct {
	WSDLHomologacao string `toml:"wsdl_homologacao"`
	WSDLProducao    string `toml:"wsdl_producao"`
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
}

// WSDLURL returns the WSDL endpoint for the configured ambiente.
func (c *Config) WSDLURL() string {
	if c.Ambiente == EnvProducao {
		return c.SOAP.WSDLProducao
	}
	return c.SOAP.WSDLHomologacao
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

	if errs := Validate(&cfg); len(errs) > 0 {
		return nil, fmt.Errorf("configuração inválida: %s", strings.Join(errs, "; "))
	}

	return &cfg, nil
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

	hasUserAuth := cfg.Autenticacao.Usuario != "" && cfg.Autenticacao.Senha != ""
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
	if cfg.SOAP.WSDLHomologacao == "" {
		errs = append(errs, "soap.wsdl_homologacao é obrigatório")
	}
	if cfg.SOAP.WSDLProducao == "" {
		errs = append(errs, "soap.wsdl_producao é obrigatório")
	}

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
