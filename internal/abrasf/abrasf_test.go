package abrasf_test

import (
	"bytes"
	"encoding/xml"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/caian-org/nfe/internal/abrasf"
	"github.com/caian-org/nfe/internal/config"
	"github.com/caian-org/nfe/internal/nota"
)

var updateGoldens = flag.Bool("update", false, "regenerate XML golden files")

func goldenPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", "golden", name)
}

// canonicalize strips inter-tag whitespace so goldens can stay pretty-printed
// in the repo while comparisons remain strict about element order/content.
var interTag = regexp.MustCompile(`>\s+<`)

func canonicalize(s string) string {
	s = strings.TrimSpace(s)
	s = interTag.ReplaceAllString(s, "><")
	return s
}

func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := goldenPath(t, name)
	if *updateGoldens {
		require.NoError(t, os.WriteFile(path, got, 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "golden %s missing — run `go test ./internal/abrasf/... -update`", name)
	if canonicalize(string(want)) != canonicalize(string(got)) {
		t.Errorf("XML mismatch for %s\n--- want\n%s\n--- got\n%s", name, want, got)
	}
}

// fixtureConfig returns a deterministic Config for golden tests.
func fixtureConfig() *config.Config {
	cfg := config.Default()
	cfg.Prestador.CNPJ = "11222333000181"
	cfg.Prestador.InscricaoMunicipal = "123456"
	cfg.Configuracoes.CodigoMunicipio = "1234567"
	cfg.Configuracoes.ProximoNumeroRPS = 7
	cfg.Configuracoes.SerieRPS = "A"
	return cfg
}

// fixtureInput returns a deterministic invoice input.
func fixtureInput() *nota.Input {
	return &nota.Input{
		Tomador: nota.Tomador{
			CNPJ:        "44555666000170",
			RazaoSocial: "TOMADOR EXEMPLO LTDA",
			Email:       "contato@tomador-exemplo.com.br",
			Telefone:    "1100000000",
			Endereco: nota.Endereco{
				Endereco:        "RUA DO TOMADOR",
				Numero:          "100",
				Complemento:     "SALA 1",
				Bairro:          "CENTRO",
				CodigoMunicipio: "7654321",
				UF:              "SP",
				CEP:             "01234000",
			},
		},
		Servico: nota.Servico{
			Discriminacao:    "DESCRIÇÃO DO SERVIÇO PRESTADO",
			ValorServicos:    15000.0,
			ItemListaServico: "0101",
			CodigoCnae:       "6201500",
			Aliquota:         2.0,
		},
		Observacoes: "Observações sobre o serviço",
	}
}

func fixtureDate() time.Time {
	return time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
}

func TestBuildRPSFromInput(t *testing.T) {
	cfg := fixtureConfig()
	in := fixtureInput()
	inf := abrasf.BuildRPS(cfg, in, fixtureDate())

	assert.Equal(t, "rps7", inf.ID)
	assert.Equal(t, 7, inf.Rps.IdentificacaoRps.Numero)
	assert.Equal(t, "00000", inf.Rps.IdentificacaoRps.Serie)
	assert.Equal(t, abrasf.TipoRPS, inf.Rps.IdentificacaoRps.Tipo)
	assert.Equal(t, "2026-05-26", inf.Rps.DataEmissao)
	assert.Equal(t, "2026-05-26", inf.Competencia)
	assert.Equal(t, 15000.0, float64(inf.Servico.Valores.ValorServicos))
	assert.Equal(t, abrasf.ISSRetidoNao, inf.Servico.IssRetido)
	assert.Equal(t, "Observações sobre o serviço", inf.InformacoesComplementares)
	require.NotNil(t, inf.TomadorServico)
	require.NotNil(t, inf.TomadorServico.IdentificacaoTomador)
	assert.Equal(t, "44555666000170", inf.TomadorServico.IdentificacaoTomador.CpfCnpj.CNPJ)
}

func TestBuildRPSDefaultObservacoes(t *testing.T) {
	cfg := fixtureConfig()
	in := fixtureInput()
	in.Observacoes = ""
	inf := abrasf.BuildRPS(cfg, in, fixtureDate())
	assert.Equal(t, "Informação Complementar Solicitada", inf.InformacoesComplementares)
}

func TestBuildRPSDefaultsCnae(t *testing.T) {
	cfg := fixtureConfig()
	in := fixtureInput()
	in.Servico.CodigoCnae = ""
	inf := abrasf.BuildRPS(cfg, in, fixtureDate())
	assert.Equal(t, "0000000", inf.Servico.CodigoCnae)
}

func TestBuildRPSFallsBackToConfigAliquota(t *testing.T) {
	cfg := fixtureConfig()
	cfg.Configuracoes.AliquotaISS = 7.0
	in := fixtureInput()
	in.Servico.Aliquota = 0
	inf := abrasf.BuildRPS(cfg, in, fixtureDate())
	require.NotNil(t, inf.Servico.Valores.Aliquota)
	assert.Equal(t, 7.0, float64(*inf.Servico.Valores.Aliquota))
}

func TestBuildRPSIssRetidoTrueZeroesValorIss(t *testing.T) {
	cfg := fixtureConfig()
	in := fixtureInput()
	in.Servico.IssRetido = true
	inf := abrasf.BuildRPS(cfg, in, fixtureDate())
	assert.Equal(t, abrasf.ISSRetidoSim, inf.Servico.IssRetido)
	require.NotNil(t, inf.Servico.Valores.ValorIss)
	assert.Equal(t, 0.0, float64(*inf.Servico.Valores.ValorIss))
}

func TestBuildRPSCPFTomador(t *testing.T) {
	cfg := fixtureConfig()
	in := fixtureInput()
	in.Tomador.CNPJ = ""
	in.Tomador.CPF = "12345678909"
	inf := abrasf.BuildRPS(cfg, in, fixtureDate())
	require.NotNil(t, inf.TomadorServico.IdentificacaoTomador)
	assert.Empty(t, inf.TomadorServico.IdentificacaoTomador.CpfCnpj.CNPJ)
	assert.Equal(t, "12345678909", inf.TomadorServico.IdentificacaoTomador.CpfCnpj.CPF)
}

func TestGerarNfseGolden(t *testing.T) {
	cfg := fixtureConfig()
	in := fixtureInput()
	inf := abrasf.BuildRPS(cfg, in, fixtureDate())
	out, err := abrasf.BuildGerarNfse(inf)
	require.NoError(t, err)

	assertGolden(t, "gerar_nfse_full.xml", out)

	// Sanity checks that double as documentation of the XML invariants.
	assert.True(t, bytes.Contains(out, []byte(`xmlns="`+abrasf.Namespace+`"`)),
		"namespace attribute must be present on root element")
	assert.True(t, bytes.Contains(out, []byte(`<Aliquota>2.0</Aliquota>`)),
		"aliquota must be formatted with exactly one decimal")
	assert.True(t, bytes.Contains(out, []byte(`<ValorServicos>15000.00</ValorServicos>`)),
		"valores must be formatted with exactly two decimals")
	assert.True(t, bytes.Contains(out, []byte(`<InfDeclaracaoPrestacaoServico Id="rps7">`)),
		"inf must carry the Id attribute for signing reference")
}

func TestGerarNfseMinimalGolden(t *testing.T) {
	cfg := fixtureConfig()
	in := fixtureInput()
	in.Tomador.Telefone = ""
	in.Tomador.Email = ""
	in.Tomador.Endereco.Complemento = ""
	in.Servico.CodigoCnae = ""
	in.Observacoes = ""
	inf := abrasf.BuildRPS(cfg, in, fixtureDate())
	out, err := abrasf.BuildGerarNfse(inf)
	require.NoError(t, err)
	assertGolden(t, "gerar_nfse_minimal.xml", out)

	assert.False(t, bytes.Contains(out, []byte("<Complemento")), "empty complemento must be omitted")
	assert.False(t, bytes.Contains(out, []byte("<Contato")), "empty contato must be omitted")
	assert.True(t, bytes.Contains(out, []byte("<CodigoCnae>0000000</CodigoCnae>")), "cnae must default to 0000000")
}

func TestGerarNfseEscapesSpecialChars(t *testing.T) {
	cfg := fixtureConfig()
	in := fixtureInput()
	in.Servico.Discriminacao = `Servic\o & < > " ' caracteres especiais`
	inf := abrasf.BuildRPS(cfg, in, fixtureDate())
	out, err := abrasf.BuildGerarNfse(inf)
	require.NoError(t, err)
	// encoding/xml escapes &, <, >; " and ' are escaped inside attributes but
	// preserved inside text content (which is XML-valid).
	assert.True(t, bytes.Contains(out, []byte("&amp;")))
	assert.True(t, bytes.Contains(out, []byte("&lt;")))
	assert.True(t, bytes.Contains(out, []byte("&gt;")))
}

func TestConsultarPorNumeroGolden(t *testing.T) {
	out, err := abrasf.BuildConsultarServicoPrestado(
		"11222333000181",
		"123456",
		abrasf.ConsultaQuery{Numero: "42"},
	)
	require.NoError(t, err)
	assertGolden(t, "consultar_por_numero.xml", out)
	assert.True(t, bytes.Contains(out, []byte("<NumeroNfse>42</NumeroNfse>")))
	assert.False(t, bytes.Contains(out, []byte("PeriodoCompetencia")))
	assert.True(t, bytes.Contains(out, []byte("<Pagina>1</Pagina>")))
}

func TestConsultarPorPeriodoGolden(t *testing.T) {
	out, err := abrasf.BuildConsultarServicoPrestado(
		"11222333000181",
		"123456",
		abrasf.ConsultaQuery{DataInicial: "2026-01-01", DataFinal: "2026-12-31"},
	)
	require.NoError(t, err)
	assertGolden(t, "consultar_por_periodo.xml", out)
	assert.True(t, bytes.Contains(out, []byte("<DataInicial>2026-01-01</DataInicial>")))
	assert.True(t, bytes.Contains(out, []byte("<DataFinal>2026-12-31</DataFinal>")))
	assert.False(t, bytes.Contains(out, []byte("NumeroNfse")))
}

func TestConsultarRequiresFilter(t *testing.T) {
	_, err := abrasf.BuildConsultarServicoPrestado("11222333000181", "123456", abrasf.ConsultaQuery{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "numero ou ambos data_inicial+data_final")
}

func TestConsultarRequiresPrestador(t *testing.T) {
	_, err := abrasf.BuildConsultarServicoPrestado("", "123456", abrasf.ConsultaQuery{Numero: "1"})
	require.Error(t, err)
	_, err = abrasf.BuildConsultarServicoPrestado("11222333000181", "", abrasf.ConsultaQuery{Numero: "1"})
	require.Error(t, err)
}

func TestCancelarNfseGolden(t *testing.T) {
	out, err := abrasf.BuildCancelarNfse(abrasf.CancelInput{
		NumeroNfse:         "100",
		CNPJ:               "11222333000181",
		InscricaoMunicipal: "123456",
		CodigoMunicipio:    "1234567",
		Codigo:             abrasf.CancelErroEmissao,
	})
	require.NoError(t, err)
	assertGolden(t, "cancelar_nfse.xml", out)
	assert.True(t, bytes.Contains(out, []byte("<CodigoCancelamento>1</CodigoCancelamento>")))
}

func TestCancelarValidatesCodigo(t *testing.T) {
	for _, codigo := range []int{0, 5, -1, 99} {
		_, err := abrasf.BuildCancelarNfse(abrasf.CancelInput{
			NumeroNfse: "1", CNPJ: "x", InscricaoMunicipal: "x", CodigoMunicipio: "x", Codigo: codigo,
		})
		require.Error(t, err, "codigo %d must be rejected", codigo)
	}
}

func TestCancelarValidatesRequiredFields(t *testing.T) {
	base := abrasf.CancelInput{
		NumeroNfse: "1", CNPJ: "x", InscricaoMunicipal: "x", CodigoMunicipio: "x", Codigo: 1,
	}
	cases := map[string]func(*abrasf.CancelInput){
		"numero":          func(c *abrasf.CancelInput) { c.NumeroNfse = "" },
		"cnpj":            func(c *abrasf.CancelInput) { c.CNPJ = "" },
		"inscricao":       func(c *abrasf.CancelInput) { c.InscricaoMunicipal = "" },
		"codigoMunicipio": func(c *abrasf.CancelInput) { c.CodigoMunicipio = "" },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			in := base
			mut(&in)
			_, err := abrasf.BuildCancelarNfse(in)
			require.Error(t, err)
		})
	}
}

func TestDec2MarshalsTwoDecimals(t *testing.T) {
	type box struct {
		XMLName xml.Name    `xml:"V"`
		Val     abrasf.Dec2 `xml:"x"`
	}
	out, err := xml.Marshal(box{Val: 1.5})
	require.NoError(t, err)
	assert.Equal(t, "<V><x>1.50</x></V>", string(out))

	out, err = xml.Marshal(box{Val: 0})
	require.NoError(t, err)
	assert.Equal(t, "<V><x>0.00</x></V>", string(out))
}

func TestBuildGerarNfseRejectsNil(t *testing.T) {
	_, err := abrasf.BuildGerarNfse(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "declaração nula")
}

func TestBuildRPSDefaultsToNowWhenTodayIsZero(t *testing.T) {
	inf := abrasf.BuildRPS(fixtureConfig(), fixtureInput(), time.Time{})
	// We don't care which exact day — just that the field looks like an ISO date.
	require.Len(t, inf.Rps.DataEmissao, 10)
	require.Equal(t, inf.Rps.DataEmissao, inf.Competencia)
}

func TestDec1MarshalsOneDecimal(t *testing.T) {
	type box struct {
		XMLName xml.Name    `xml:"V"`
		Val     abrasf.Dec1 `xml:"x"`
	}
	out, err := xml.Marshal(box{Val: 2})
	require.NoError(t, err)
	assert.Equal(t, "<V><x>2.0</x></V>", string(out))

	out, err = xml.Marshal(box{Val: 5.75})
	require.NoError(t, err)
	// Note: TS toFixed(1) of 5.75 yields "5.8" via banker's-round-ish; Go's
	// FormatFloat rounds half-to-even and gives "5.8" too — confirm.
	assert.Equal(t, "<V><x>5.8</x></V>", string(out))
}
