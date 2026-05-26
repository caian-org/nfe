package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	mathrand "math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/caian-org/nfe/internal/abrasf"
	"github.com/caian-org/nfe/internal/config"
	"github.com/caian-org/nfe/internal/nota"
	"github.com/caian-org/nfe/internal/service"
	"github.com/caian-org/nfe/internal/xmlsig"
)

type fakeSOAP struct {
	lastAction string
	lastBody   []byte
	response   []byte
	err        error
}

func (f *fakeSOAP) Call(ctx context.Context, action string, body []byte) ([]byte, error) {
	f.lastAction = action
	f.lastBody = body
	return f.response, f.err
}

func makeCert(t *testing.T) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(mathrand.Int63()),
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

func fixtureService(t *testing.T, withSigner bool) (*service.Service, *fakeSOAP) {
	t.Helper()
	cfg := config.Default()
	cfg.Prestador.CNPJ = "11222333000181"
	cfg.Prestador.InscricaoMunicipal = "123456"

	opts := service.Options{
		Config: cfg,
		SOAP:   &fakeSOAP{},
		Now:    func() time.Time { return time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC) },
	}
	if withSigner {
		cert, key := makeCert(t)
		signer, err := xmlsig.NewSigner(cert, key)
		require.NoError(t, err)
		opts.Signer = signer
	}
	svc, err := service.New(opts)
	require.NoError(t, err)
	return svc, opts.SOAP.(*fakeSOAP)
}

func fixtureInput() *nota.Input {
	return &nota.Input{
		Tomador: nota.Tomador{
			CNPJ:        "44555666000170",
			RazaoSocial: "TOMADOR LTDA",
			Endereco: nota.Endereco{
				Endereco:        "R",
				Numero:          "1",
				Bairro:          "B",
				CodigoMunicipio: "7654321",
				UF:              "SP",
				CEP:             "01000000",
			},
		},
		Servico: nota.Servico{
			Discriminacao:    "Serviço de teste",
			ValorServicos:    1500.0,
			ItemListaServico: "0101",
			Aliquota:         5.0,
		},
	}
}

func wrapSOAPResponse(body string) []byte {
	return []byte(`<?xml version="1.0"?><soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>` + body + `</soap:Body></soap:Envelope>`)
}

func TestEmitSuccessBumpsCounter(t *testing.T) {
	svc, fake := fixtureService(t, false)
	fake.response = wrapSOAPResponse(`<GerarNfseResposta xmlns="http://www.abrasf.org.br/nfse.xsd"><ListaNfse><CompNfse><Nfse><InfNfse><Numero>1001</Numero></InfNfse></Nfse></CompNfse></ListaNfse></GerarNfseResposta>`)

	initialCounter := svc.Config().Configuracoes.ProximoNumeroRPS

	res, err := svc.Emit(context.Background(), fixtureInput())
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, initialCounter, res.NumeroRPS)
	assert.Equal(t, "1001", res.NumeroNFSe)
	assert.Equal(t, initialCounter+1, svc.Config().Configuracoes.ProximoNumeroRPS,
		"counter must be bumped after successful emit")
	assert.Equal(t, "GerarNfse", fake.lastAction)
	assert.Contains(t, string(fake.lastBody), "GerarNfseEnvio")
}

func TestEmitRejectsInvalidInput(t *testing.T) {
	svc, _ := fixtureService(t, false)
	in := fixtureInput()
	in.Servico.ValorServicos = 0
	_, err := svc.Emit(context.Background(), in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "valor dos serviços")
}

func TestEmitSurfacesMensagemRetorno(t *testing.T) {
	svc, fake := fixtureService(t, false)
	fake.response = wrapSOAPResponse(`<GerarNfseResposta><ListaMensagemRetorno><MensagemRetorno><Codigo>E001</Codigo><Mensagem>boom</Mensagem></MensagemRetorno></ListaMensagemRetorno></GerarNfseResposta>`)

	initialCounter := svc.Config().Configuracoes.ProximoNumeroRPS
	res, err := svc.Emit(context.Background(), fixtureInput())
	require.Error(t, err)
	require.NotNil(t, res)
	var me *service.MessagesError
	require.ErrorAs(t, err, &me)
	assert.Equal(t, "emit", me.Action)
	require.Len(t, me.Mensagens, 1)
	assert.Equal(t, "E001", me.Mensagens[0].Codigo)
	assert.Equal(t, initialCounter, svc.Config().Configuracoes.ProximoNumeroRPS,
		"counter must NOT be bumped when the WS returned an error")
}

func TestEmitNetworkErrorDoesNotBumpCounter(t *testing.T) {
	svc, fake := fixtureService(t, false)
	fake.err = errors.New("connection refused")
	initialCounter := svc.Config().Configuracoes.ProximoNumeroRPS
	_, err := svc.Emit(context.Background(), fixtureInput())
	require.Error(t, err)
	assert.Equal(t, initialCounter, svc.Config().Configuracoes.ProximoNumeroRPS)
}

func TestEmitSignsWhenSignerProvided(t *testing.T) {
	svc, fake := fixtureService(t, true)
	fake.response = wrapSOAPResponse(`<GerarNfseResposta><ListaNfse><CompNfse><Nfse><InfNfse><Numero>1</Numero></InfNfse></Nfse></CompNfse></ListaNfse></GerarNfseResposta>`)
	_, err := svc.Emit(context.Background(), fixtureInput())
	require.NoError(t, err)
	assert.Contains(t, string(fake.lastBody), "<Signature", "signed XML must contain a Signature element")
}

func TestEmitDryRunDoesNotCallSoapOrBumpCounter(t *testing.T) {
	svc, fake := fixtureService(t, true)
	initialCounter := svc.Config().Configuracoes.ProximoNumeroRPS
	res, err := svc.EmitDryRun(fixtureInput())
	require.NoError(t, err)
	assert.NotEmpty(t, res.SignedXML)
	assert.NotEmpty(t, res.UnsignedXML)
	assert.Empty(t, fake.lastBody, "SOAP must not be called in dry-run")
	assert.Equal(t, initialCounter, svc.Config().Configuracoes.ProximoNumeroRPS)
}

func TestEmitDryRunValidates(t *testing.T) {
	svc, _ := fixtureService(t, false)
	in := fixtureInput()
	in.Tomador.RazaoSocial = ""
	_, err := svc.EmitDryRun(in)
	require.Error(t, err)
}

func TestQueryByNumero(t *testing.T) {
	svc, fake := fixtureService(t, false)
	fake.response = wrapSOAPResponse(`<ConsultarNfseResposta><ListaNfse><CompNfse><Nfse><InfNfse><Numero>1</Numero></InfNfse></Nfse></CompNfse></ListaNfse></ConsultarNfseResposta>`)

	res, err := svc.Query(context.Background(), abrasf.ConsultaQuery{Numero: "42"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "ConsultarNfseServicoPrestado", fake.lastAction)
	assert.Contains(t, string(fake.lastBody), "<NumeroNfse>42</NumeroNfse>")
}

func TestQuerySurfacesErrors(t *testing.T) {
	svc, fake := fixtureService(t, false)
	fake.response = wrapSOAPResponse(`<r><MensagemRetorno><Codigo>E1</Codigo><Mensagem>m</Mensagem></MensagemRetorno></r>`)
	_, err := svc.Query(context.Background(), abrasf.ConsultaQuery{Numero: "1"})
	require.Error(t, err)
	var me *service.MessagesError
	require.ErrorAs(t, err, &me)
	assert.Equal(t, "query", me.Action)
}

func TestQueryRequiresFilter(t *testing.T) {
	svc, _ := fixtureService(t, false)
	_, err := svc.Query(context.Background(), abrasf.ConsultaQuery{})
	require.Error(t, err)
}

func TestCancelSuccess(t *testing.T) {
	svc, fake := fixtureService(t, false)
	fake.response = wrapSOAPResponse(`<CancelarNfseResposta><Confirmacao/></CancelarNfseResposta>`)

	res, err := svc.Cancel(context.Background(), "100", abrasf.CancelErroEmissao)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "CancelarNfse", fake.lastAction)
	assert.Equal(t, "100", res.NumeroNFSe)
}

func TestCancelSigned(t *testing.T) {
	svc, fake := fixtureService(t, true)
	fake.response = wrapSOAPResponse(`<CancelarNfseResposta><Confirmacao/></CancelarNfseResposta>`)
	_, err := svc.Cancel(context.Background(), "100", abrasf.CancelErroEmissao)
	require.NoError(t, err)
	assert.Contains(t, string(fake.lastBody), "<Signature")
}

func TestCancelValidatesCodigo(t *testing.T) {
	svc, _ := fixtureService(t, false)
	_, err := svc.Cancel(context.Background(), "100", 99)
	require.Error(t, err)
}

func TestServiceNewRequiresConfig(t *testing.T) {
	_, err := service.New(service.Options{})
	require.Error(t, err)
}
