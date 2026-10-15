package dps_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"flag"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/caian-org/nfe/internal/config"
	"github.com/caian-org/nfe/internal/dps"
	"github.com/caian-org/nfe/internal/nota"
	"github.com/caian-org/nfe/internal/xmlsig"
)

var updateGoldens = flag.Bool("update", false, "regenerate XML golden files")

var interTag = regexp.MustCompile(`>\s+<`)

func canonicalize(s string) string {
	return interTag.ReplaceAllString(strings.TrimSpace(s), "><")
}

func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "golden", name)
	if *updateGoldens {
		require.NoError(t, os.WriteFile(path, got, 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "golden %s missing — run `go test ./internal/dps/... -update`", name)
	if canonicalize(string(want)) != canonicalize(string(got)) {
		t.Errorf("XML mismatch for %s\n--- want\n%s\n--- got\n%s", name, want, got)
	}
}

func fixtureConfig() *config.Config {
	cfg := config.Default()
	cfg.Prestador.CNPJ = "11222333000181"
	cfg.Prestador.InscricaoMunicipal = "123456"
	cfg.Configuracoes.CodigoMunicipio = "3516309"
	cfg.Configuracoes.ProximoNumeroRPS = 7
	cfg.Configuracoes.ProximoNumeroNFSe = 3
	cfg.Nacional.VersaoAplicacao = "nfe-test"
	return cfg
}

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
				CodigoMunicipio: "3550308",
				Municipio:       "SAO PAULO",
				UF:              "SP",
				CEP:             "01234000",
			},
		},
		Servico: nota.Servico{
			Discriminacao:            "DESCRIÇÃO DO SERVIÇO PRESTADO",
			ValorServicos:            15000.0,
			ItemListaServico:         "0101",
			CodigoCnae:               "6201500",
			Aliquota:                 2.0,
			CodigoTributacaoNacional: "010101",
		},
		CNBS:        "115022000",
		Observacoes: "Observações sobre o serviço",
	}
}

func fixtureDate() time.Time {
	loc := time.FixedZone("BRT", -3*3600)
	return time.Date(2026, 10, 15, 9, 30, 0, 0, loc)
}

func TestBuildIDAndHeader(t *testing.T) {
	d, err := dps.Build(fixtureConfig(), fixtureInput(), fixtureDate())
	require.NoError(t, err)

	assert.Equal(t, "DPS"+"3516309"+"2"+"11222333000181"+"00001"+"000000000000007", d.InfDPS.ID)
	assert.Len(t, d.InfDPS.ID, 45)
	assert.Equal(t, 2, d.InfDPS.TpAmb, "homologacao maps to tpAmb 2")
	assert.Equal(t, "2026-10-15T09:30:00-03:00", d.InfDPS.DhEmi)
	assert.Equal(t, "2026-10-15", d.InfDPS.DCompet)
	assert.Equal(t, "1", d.InfDPS.Serie)
	assert.Equal(t, "7", d.InfDPS.NDPS)
	assert.Equal(t, "3516309", d.InfDPS.CLocEmi)
	assert.Equal(t, "3516309", d.InfDPS.Serv.LocPrest.CLocPrestacao)
	assert.Equal(t, 1, d.InfDPS.Prest.RegTrib.OpSimpNac)
	assert.Nil(t, d.InfDPS.Valores.Trib.TribMun.PAliq, "pAliq is omitted for não optante in the issuing município")
	require.NotNil(t, d.InfDPS.IBSCBS)
	assert.Equal(t, "100301", d.InfDPS.IBSCBS.CIndOp)
	assert.Equal(t, "000", d.InfDPS.IBSCBS.Valores.Trib.GIBSCBS.CST)
	assert.Equal(t, "000001", d.InfDPS.IBSCBS.Valores.Trib.GIBSCBS.CClassTrib)
}

func TestBuildIDPadsSerieAndNumber(t *testing.T) {
	cfg := fixtureConfig()
	cfg.Nacional.SerieDPS = "00012"
	cfg.Configuracoes.ProximoNumeroRPS = 123456
	d, err := dps.Build(cfg, fixtureInput(), fixtureDate())
	require.NoError(t, err)
	assert.Equal(t, "12", d.InfDPS.Serie, "serie travels without leading zeros")
	assert.True(t, strings.HasSuffix(d.InfDPS.ID, "00012"+"000000000123456"))
}

func TestBuildGolden(t *testing.T) {
	d, err := dps.Build(fixtureConfig(), fixtureInput(), fixtureDate())
	require.NoError(t, err)
	out, err := dps.Marshal(d)
	require.NoError(t, err)
	assertGolden(t, "dps_ibscbs.xml", out)

	s := string(out)
	assert.True(t, strings.HasPrefix(s, `<?xml version="1.0" encoding="UTF-8"?>`))
	assert.Contains(t, s, `<DPS xmlns="http://www.sped.fazenda.gov.br/nfse" versao="1.01">`)
	assert.Contains(t, s, `<infDPS Id="`+d.InfDPS.ID+`">`)
	assert.Contains(t, s, `<vServ>15000.00</vServ>`)
	assert.Contains(t, s, `<cNBS>115022000</cNBS>`)
	assert.Contains(t, s, `<xInfComp>Observações sobre o serviço</xInfComp>`)
	assert.Contains(t, s, `</valores><IBSCBS><finNFSe>0</finNFSe><cIndOp>100301</cIndOp><indDest>0</indDest>`)
	assert.NotContains(t, s, `<pAliq>`)
	assert.NotContains(t, s, `<xCpl></xCpl>`)
}

func TestBuildMinimalGolden(t *testing.T) {
	cfg := fixtureConfig()
	cfg.IBSCBS = nil
	in := fixtureInput()
	in.CNBS = ""
	in.Observacoes = ""
	in.Tomador.Telefone = ""
	in.Tomador.Email = ""
	in.Tomador.Endereco.Complemento = ""
	in.Tomador.CNPJ = ""
	in.Tomador.CPF = "12345678909"
	d, err := dps.Build(cfg, in, fixtureDate())
	require.NoError(t, err)
	out, err := dps.Marshal(d)
	require.NoError(t, err)
	assertGolden(t, "dps_minimal.xml", out)

	s := string(out)
	assert.NotContains(t, s, "<IBSCBS>")
	assert.NotContains(t, s, "<cNBS>")
	assert.NotContains(t, s, "<infoCompl>")
	assert.NotContains(t, s, "<fone>")
	assert.Contains(t, s, "<CPF>12345678909</CPF>")
}

func TestBuildPAliqRules(t *testing.T) {
	cfg := fixtureConfig()
	in := fixtureInput()
	in.Servico.LocalPrestacao = "3550308"
	d, err := dps.Build(cfg, in, fixtureDate())
	require.NoError(t, err)
	require.NotNil(t, d.InfDPS.Valores.Trib.TribMun.PAliq, "service outside the issuing município requires pAliq")
	assert.Equal(t, 2.0, float64(*d.InfDPS.Valores.Trib.TribMun.PAliq))

	cfg.Nacional.OpSimpNac = config.OpSimpNacMEEPP
	in = fixtureInput()
	in.Servico.Aliquota = 0
	cfg.Configuracoes.AliquotaISS = 3.5
	d, err = dps.Build(cfg, in, fixtureDate())
	require.NoError(t, err)
	require.NotNil(t, d.InfDPS.Valores.Trib.TribMun.PAliq)
	assert.Equal(t, 3.5, float64(*d.InfDPS.Valores.Trib.TribMun.PAliq))
	out, err := dps.Marshal(d)
	require.NoError(t, err)
	assert.Contains(t, string(out), "<pAliq>3.50</pAliq>")
}

func TestBuildIssRetido(t *testing.T) {
	in := fixtureInput()
	in.Servico.IssRetido = true
	d, err := dps.Build(fixtureConfig(), in, fixtureDate())
	require.NoError(t, err)
	assert.Equal(t, dps.TpRetISSQNRetidoTomador, d.InfDPS.Valores.Trib.TribMun.TpRetISSQN)
}

func TestBuildNotaOverridesIBSCBS(t *testing.T) {
	in := fixtureInput()
	in.IBSCBS = &nota.IBSCBS{CST: "200", CodigoClassificacaoTributaria: "200003"}
	d, err := dps.Build(fixtureConfig(), in, fixtureDate())
	require.NoError(t, err)
	assert.Equal(t, "200", d.InfDPS.IBSCBS.Valores.Trib.GIBSCBS.CST)
	assert.Equal(t, "200003", d.InfDPS.IBSCBS.Valores.Trib.GIBSCBS.CClassTrib)
	assert.Equal(t, "100301", d.InfDPS.IBSCBS.CIndOp, "unset override keeps the config value")
}

func TestBuildErrors(t *testing.T) {
	t.Run("missing cTribNac", func(t *testing.T) {
		in := fixtureInput()
		in.Servico.CodigoTributacaoNacional = ""
		_, err := dps.Build(fixtureConfig(), in, fixtureDate())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "codigo_tributacao_nacional")
	})
	t.Run("IBSCBS without cNBS", func(t *testing.T) {
		in := fixtureInput()
		in.CNBS = ""
		_, err := dps.Build(fixtureConfig(), in, fixtureDate())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cnbs é obrigatório")
	})
	t.Run("partial tomador address", func(t *testing.T) {
		in := fixtureInput()
		in.Tomador.Endereco.CEP = ""
		_, err := dps.Build(fixtureConfig(), in, fixtureDate())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "endereço do tomador incompleto")
	})
	t.Run("no nacional section", func(t *testing.T) {
		cfg := fixtureConfig()
		cfg.Nacional = nil
		_, err := dps.Build(cfg, fixtureInput(), fixtureDate())
		require.Error(t, err)
	})
}

func TestBuildOmitsEmptyTomadorAddress(t *testing.T) {
	in := fixtureInput()
	in.Tomador.Endereco = nota.Endereco{}
	d, err := dps.Build(fixtureConfig(), in, fixtureDate())
	require.NoError(t, err)
	assert.Nil(t, d.InfDPS.Toma.End)
}

func makeCert(t *testing.T) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "TEST"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert, key
}

func TestSignedDPSPlacesSignatureAfterInfDPS(t *testing.T) {
	d, err := dps.Build(fixtureConfig(), fixtureInput(), fixtureDate())
	require.NoError(t, err)
	unsigned, err := dps.Marshal(d)
	require.NoError(t, err)

	cert, key := makeCert(t)
	signer, err := xmlsig.NewSigner(cert, key)
	require.NoError(t, err)
	signed, err := signer.SignSibling(unsigned, "infDPS", d.InfDPS.ID)
	require.NoError(t, err)

	doc := etree.NewDocument()
	require.NoError(t, doc.ReadFromBytes(signed))
	root := doc.Root()
	require.Equal(t, "DPS", root.Tag)
	children := root.ChildElements()
	require.Len(t, children, 2)
	assert.Equal(t, "infDPS", children[0].Tag)
	assert.Equal(t, "Signature", children[1].Tag)
	ref := children[1].FindElement(".//Reference")
	require.NotNil(t, ref)
	assert.Equal(t, "#"+d.InfDPS.ID, ref.SelectAttrValue("URI", ""))
}

func TestParseNFSe(t *testing.T) {
	raw := []byte(`<?xml version="1.0"?><NFSe xmlns="http://www.sped.fazenda.gov.br/nfse" versao="1.01"><infNFSe Id="NFS35163092211222333000181000000000012202610123456789"><xLocEmi>FRANCO DA ROCHA</xLocEmi><nNFSe>12</nNFSe><dhProc>2026-10-15T09:31:00-03:00</dhProc><DPS><infDPS Id="DPS1"><dhEmi>2026-10-15T09:30:00-03:00</dhEmi><nDPS>7</nDPS><toma><CNPJ>44555666000170</CNPJ><xNome>TOMADOR EXEMPLO LTDA</xNome></toma><valores><vServPrest><vServ>15000.00</vServ></vServPrest></valores></infDPS></DPS></infNFSe></NFSe>`)
	n, err := dps.ParseNFSe(raw)
	require.NoError(t, err)
	assert.Equal(t, "12", n.Numero)
	assert.Equal(t, "35163092211222333000181000000000012202610123456789", n.ChaveAcesso)
	assert.Equal(t, "2026-10-15T09:31:00-03:00", n.DataProcesso)
	assert.Equal(t, "2026-10-15T09:30:00-03:00", n.DataEmissao)
	assert.Equal(t, "15000.00", n.ValorServicos)
	assert.Equal(t, "TOMADOR EXEMPLO LTDA", n.Tomador)
	assert.Equal(t, "7", n.NumeroDPS)

	_, err = dps.ParseNFSe([]byte("<x/>"))
	require.Error(t, err)
	_, err = dps.ParseNFSe(nil)
	require.Error(t, err)
}
