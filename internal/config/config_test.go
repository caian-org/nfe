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
	timeout, interval, err := cfg.Configuracoes.ConfirmDurations()
	require.NoError(t, err)
	assert.Equal(t, "2m0s", timeout.String())
	assert.Equal(t, "5s", interval.String())
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

func TestDirectEndpointSelection(t *testing.T) {
	cfg := config.Default()
	cfg.SOAP.EndpointHomologacao = "https://soap-homologacao.example.test/ws/nfs"
	cfg.SOAP.EndpointProducao = "https://soap-producao.example.test/ws/nfs"
	cfg.SOAP.NamespaceHomologacao = "urn:hml"
	cfg.SOAP.NamespaceProducao = "urn:prd"

	assert.Equal(t, "https://soap-homologacao.example.test/ws/nfs", cfg.EndpointURL())
	assert.Equal(t, "urn:hml", cfg.MethodNamespace())
	assert.Equal(t, config.DefaultSOAPActionPrefix, cfg.SOAPActionPrefix())

	cfg.Ambiente = config.EnvProducao
	assert.Equal(t, "https://soap-producao.example.test/ws/nfs", cfg.EndpointURL())
	assert.Equal(t, "urn:prd", cfg.MethodNamespace())
}

func TestDirectEndpointFallsBackNamespaceToEndpoint(t *testing.T) {
	cfg := config.Default()
	cfg.SOAP.EndpointHomologacao = "https://soap.example.test/ws/nfs"

	assert.Equal(t, cfg.SOAP.EndpointHomologacao, cfg.MethodNamespace())
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

func TestLoadAppliesConfirmationDefaults(t *testing.T) {
	body := `ambiente = "homologacao"

[soap]
wsdl_homologacao = "https://teste.exemplo.com.br/abrasf/ws/nfs?wsdl"
wsdl_producao = "https://producao.exemplo.com.br/abrasf/ws/nfs?wsdl"

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

[autenticacao]
usuario = "alice"
senha = "s3cret"

[configuracoes]
serie_rps = "A"
proximo_numero_rps = 1
codigo_municipio = "1234567"
aliquota_iss = 5.0
`
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, config.DefaultConfirmTimeout, cfg.Configuracoes.ConfirmTimeout)
	assert.Equal(t, config.DefaultConfirmInterval, cfg.Configuracoes.ConfirmInterval)
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
			name: "missing wsdl homologacao in soap mode",
			fix: func(c *config.Config) {
				c.Nacional = nil
				c.SOAP.WSDLHomologacao = ""
			},
			want: "soap.wsdl_homologacao",
		},
		{
			name: "wsdl homologacao can be replaced by endpoint",
			fix: func(c *config.Config) {
				c.Nacional = nil
				c.SOAP.WSDLHomologacao = ""
				c.SOAP.EndpointHomologacao = "https://soap.example.test/ws/nfs"
			},
			want: "",
		},
		{
			name: "nacional mode does not need soap",
			fix: func(c *config.Config) {
				c.SOAP = config.SOAP{}
			},
			want: "",
		},
		{
			name: "nacional serie must be numeric",
			fix:  func(c *config.Config) { c.Nacional.SerieDPS = "E" },
			want: "nacional.serie_dps",
		},
		{
			name: "nacional needs senha",
			fix: func(c *config.Config) {
				c.Autenticacao.Senha = ""
				c.Autenticacao.Certificado = &config.Certificado{Path: "c.pfx", Senha: "x"}
			},
			want: "autenticacao.senha",
		},
		{
			name: "nacional invalid op_simp_nac",
			fix:  func(c *config.Config) { c.Nacional.OpSimpNac = 7 },
			want: "nacional.op_simp_nac",
		},
		{
			name: "nacional nNFSe counter must be positive",
			fix:  func(c *config.Config) { c.Configuracoes.ProximoNumeroNFSe = 0 },
			want: "proximo_numero_nfse",
		},
		{
			name: "ibscbs missing rate",
			fix:  func(c *config.Config) { c.IBSCBS.AliquotaCBS = nil },
			want: "ibscbs.aliquota_cbs é obrigatória",
		},
		{
			name: "ibscbs rate out of range",
			fix:  func(c *config.Config) { v := 150.0; c.IBSCBS.AliquotaIBSUF = &v },
			want: "ibscbs.aliquota_ibs_uf deve estar entre 0 e 100",
		},
		{
			name: "ibscbs bad cst",
			fix:  func(c *config.Config) { c.IBSCBS.CST = "0" },
			want: "ibscbs.cst",
		},
		{
			name: "ibscbs bad class trib",
			fix:  func(c *config.Config) { c.IBSCBS.CodigoClassificacaoTributaria = "1" },
			want: "ibscbs.codigo_classificacao_tributaria",
		},
		{
			name: "invalid confirm timeout",
			fix:  func(c *config.Config) { c.Configuracoes.ConfirmTimeout = "nada" },
			want: "configuracoes.confirm_timeout",
		},
		{
			name: "zero confirm interval",
			fix:  func(c *config.Config) { c.Configuracoes.ConfirmInterval = "0s" },
			want: "configuracoes.confirm_interval",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			tc.fix(cfg)
			errs := config.Validate(cfg)
			if tc.want == "" {
				require.Empty(t, errs, "expected config to be valid")
				return
			}
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

func TestNacionalHelpers(t *testing.T) {
	cfg := config.Default()
	assert.True(t, cfg.IsNacional())
	assert.Equal(t, config.DefaultNacionalURLHomologacao, cfg.NacionalURL())
	assert.Equal(t, 2, cfg.TpAmb())
	assert.True(t, cfg.AssinarDPS())
	assert.Equal(t, "usuario", cfg.NacionalUsuario())

	cfg.Ambiente = config.EnvProducao
	cfg.Nacional.URLProducao = "https://prod.example.test/"
	assert.Equal(t, "https://prod.example.test", cfg.NacionalURL())
	assert.Equal(t, 1, cfg.TpAmb())

	cfg.Autenticacao.Usuario = ""
	assert.Equal(t, cfg.Prestador.InscricaoMunicipal, cfg.NacionalUsuario())

	off := false
	cfg.Nacional.AssinarDPS = &off
	assert.False(t, cfg.AssinarDPS())

	cfg.Nacional = nil
	assert.False(t, cfg.IsNacional())
	assert.Equal(t, "", cfg.NacionalURL())
	assert.False(t, cfg.AssinarDPS())
}

func TestNacionalRoundTripAndDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `ambiente = "homologacao"

[nacional]
serie_dps = "5"

[prestador]
cnpj = "11222333000181"
inscricao_municipal = "123456"
razao_social = "EXEMPLO LTDA"

[prestador.endereco]
endereco = "RUA"
numero = "1"
bairro = "B"
codigo_municipio = "3516309"
uf = "SP"
cep = "01234000"

[autenticacao]
senha = "segredo"

[configuracoes]
serie_rps = "A"
proximo_numero_rps = 1
codigo_municipio = "3516309"
aliquota_iss = 2.0

[ibscbs]
cst = "000"
codigo_classificacao_tributaria = "000001"
codigo_indicador_operacao = "100301"
aliquota_ibs_uf = 0.1
aliquota_ibs_municipal = 0.0
aliquota_cbs = 0.9
`
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	cfg, err := config.Load(path)
	require.NoError(t, err)
	require.NotNil(t, cfg.Nacional)
	assert.Equal(t, config.DefaultNacionalURLProducao, cfg.Nacional.URLProducao)
	assert.Equal(t, "5", cfg.Nacional.SerieDPS)
	assert.Equal(t, config.OpSimpNacNaoOptante, cfg.Nacional.OpSimpNac)
	assert.Equal(t, 1, cfg.Configuracoes.ProximoNumeroNFSe)
	assert.Equal(t, "123456", cfg.NacionalUsuario())
	require.NotNil(t, cfg.IBSCBS.AliquotaIBSMunicipal)
	assert.Equal(t, 0.0, *cfg.IBSCBS.AliquotaIBSMunicipal)

	require.NoError(t, config.Save(path, cfg))
	again, err := config.Load(path)
	require.NoError(t, err)
	require.NotNil(t, again.IBSCBS.AliquotaIBSMunicipal)
	assert.Equal(t, 0.0, *again.IBSCBS.AliquotaIBSMunicipal)
	assert.Equal(t, cfg.Nacional.URLHomologacao, again.Nacional.URLHomologacao)
}
