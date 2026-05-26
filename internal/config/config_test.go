package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/caian-org/nfe/internal/config"
)

func testdata(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", name)
}

func TestLoadFull(t *testing.T) {
	t.Setenv("NFE_USUARIO", "bob")
	t.Setenv("NFE_SENHA", "hunter2")

	cfg, err := config.Load(testdata(t, "config_full.toml"))
	require.NoError(t, err)

	assert.Equal(t, "homologacao", cfg.Ambiente)
	assert.False(t, cfg.IsProducao())
	assert.Contains(t, cfg.WSDLURL(), "teste.exemplo.com.br")
	assert.Equal(t, "11222333000181", cfg.Prestador.CNPJ)
	assert.Equal(t, "bob", cfg.Autenticacao.Usuario)
	assert.Equal(t, "hunter2", cfg.Autenticacao.Senha)
	require.NotNil(t, cfg.Autenticacao.Certificado)
	assert.Equal(t, "/tmp/certs/empresa.pfx", cfg.Autenticacao.Certificado.Path)
}

func TestLoadNoCert(t *testing.T) {
	cfg, err := config.Load(testdata(t, "config_no_cert.toml"))
	require.NoError(t, err)
	assert.Nil(t, cfg.Autenticacao.Certificado)
	assert.Equal(t, "alice", cfg.Autenticacao.Usuario)
	assert.Equal(t, 42, cfg.Configuracoes.ProximoNumeroRPS)
}

func TestProducaoSelection(t *testing.T) {
	t.Setenv("NFE_USUARIO", "x")
	t.Setenv("NFE_SENHA", "x")
	cfg, err := config.Load(testdata(t, "config_full.toml"))
	require.NoError(t, err)

	cfg.Ambiente = config.EnvProducao
	assert.True(t, cfg.IsProducao())
	assert.Contains(t, cfg.WSDLURL(), "producao.exemplo.com.br")
	assert.NotContains(t, cfg.WSDLURL(), "teste")
}

func TestLoadMissing(t *testing.T) {
	_, err := config.Load("/no/such/path.toml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "não encontrado")
}

// TestLoadResolvesRelativeCertPath verifies that relative cert paths in the
// config file are resolved against the config file's directory rather than
// the current working directory. Absolute paths are passed through unchanged
// and no-cert configs return "".
func TestLoadResolvesRelativeCertPath(t *testing.T) {
	writeConfig := func(t *testing.T, dir, certPath string) string {
		t.Helper()
		body := `ambiente = "homologacao"

[soap]
wsdl_homologacao = "https://teste.exemplo.com.br/abrasf/ws/nfs?wsdl"
wsdl_producao    = "https://producao.exemplo.com.br/abrasf/ws/nfs?wsdl"

[prestador]
cnpj = "11222333000181"
inscricao_municipal = "123456"
razao_social = "EXEMPLO LTDA"

[prestador.endereco]
endereco = "RUA EXEMPLO"
numero = "100"
bairro = "CENTRO"
codigo_municipio = "1234567"
uf = "SP"
cep = "01234000"

[autenticacao.certificado]
path = "` + certPath + `"
senha = "hunter2"

[configuracoes]
serie_rps = "A"
proximo_numero_rps = 1
codigo_municipio = "1234567"
aliquota_iss = 5.0
`
		p := filepath.Join(dir, "config.toml")
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
		return p
	}

	t.Run("relative path resolves against config dir", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := writeConfig(t, dir, "./cert.pfx")

		cfg, err := config.Load(cfgPath)
		require.NoError(t, err)

		// Struct field stays as the user wrote it — Save() must round-trip
		// the portable relative form back to disk.
		assert.Equal(t, "./cert.pfx", cfg.Autenticacao.Certificado.Path)

		// The resolved path is anchored at the config file's directory.
		assert.Equal(t, filepath.Join(dir, "cert.pfx"), cfg.CertificatePath())
	})

	t.Run("absolute path passes through unchanged", func(t *testing.T) {
		dir := t.TempDir()
		abs := "/tmp/certs/empresa.pfx"
		cfgPath := writeConfig(t, dir, abs)

		cfg, err := config.Load(cfgPath)
		require.NoError(t, err)

		assert.Equal(t, abs, cfg.Autenticacao.Certificado.Path)
		assert.Equal(t, abs, cfg.CertificatePath())
	})

	t.Run("no cert section returns empty string", func(t *testing.T) {
		cfg, err := config.Load(testdata(t, "config_no_cert.toml"))
		require.NoError(t, err)
		assert.Equal(t, "", cfg.CertificatePath())
	})
}

func TestLoadEnvVarKeptWhenUnset(t *testing.T) {
	// Without setting NFE_USUARIO / NFE_SENHA, the literal placeholder must
	// survive parsing so validation can complain about the missing values.
	cfg, err := config.Load(testdata(t, "config_full.toml"))
	require.NoError(t, err)
	assert.Equal(t, "${NFE_USUARIO}", cfg.Autenticacao.Usuario)
	assert.Equal(t, "${NFE_SENHA}", cfg.Autenticacao.Senha)
}

func TestSaveRoundTrip(t *testing.T) {
	t.Setenv("NFE_USUARIO", "x")
	t.Setenv("NFE_SENHA", "x")
	src, err := config.Load(testdata(t, "config_full.toml"))
	require.NoError(t, err)

	tmp := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, config.Save(tmp, src))

	// Load it back — no env vars are expanded a second time because Save
	// writes the literal values that Load already expanded.
	got, err := config.Load(tmp)
	require.NoError(t, err)
	assert.Equal(t, src.Prestador.CNPJ, got.Prestador.CNPJ)
	assert.Equal(t, src.Autenticacao.Usuario, got.Autenticacao.Usuario)
}

func TestSaveIsAtomic(t *testing.T) {
	cfg := config.Default()
	cfg.Autenticacao.Usuario = "u"
	cfg.Autenticacao.Senha = "p"

	dir := t.TempDir()
	path := filepath.Join(dir, "c.toml")
	require.NoError(t, config.Save(path, cfg))

	entries, err := filepath.Glob(filepath.Join(dir, ".config-*.toml.tmp"))
	require.NoError(t, err)
	assert.Empty(t, entries, "atomic Save must not leave temp files behind")
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		fix  func(*config.Config)
		want string
	}{
		{
			name: "missing ambiente",
			fix:  func(c *config.Config) { c.Ambiente = "" },
			want: `ambiente deve ser "homologacao" ou "producao"`,
		},
		{
			name: "missing cnpj prestador",
			fix:  func(c *config.Config) { c.Prestador.CNPJ = "" },
			want: "CNPJ do prestador é obrigatório",
		},
		{
			name: "no auth at all",
			fix: func(c *config.Config) {
				c.Autenticacao.Usuario = ""
				c.Autenticacao.Senha = ""
				c.Autenticacao.Certificado = nil
			},
			want: "autenticação é obrigatória",
		},
		{
			name: "cert with wrong extension",
			fix: func(c *config.Config) {
				c.Autenticacao.Certificado = &config.Certificado{Path: "cert.crt", Senha: "x"}
			},
			want: ".p12 ou .pfx",
		},
		{
			name: "missing codigo municipio",
			fix:  func(c *config.Config) { c.Configuracoes.CodigoMunicipio = "" },
			want: "código do município é obrigatório",
		},
		{
			name: "missing wsdl homologacao",
			fix:  func(c *config.Config) { c.SOAP.WSDLHomologacao = "" },
			want: "soap.wsdl_homologacao",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			tc.fix(cfg)
			errs := config.Validate(cfg)
			require.NotEmpty(t, errs, "expected validation errors")
			joined := ""
			for _, e := range errs {
				joined += e + "\n"
			}
			assert.Contains(t, joined, tc.want)
		})
	}
}

func TestDefaultPassesValidation(t *testing.T) {
	errs := config.Validate(config.Default())
	require.Empty(t, errs, "default config must be valid: %v", errs)
}
