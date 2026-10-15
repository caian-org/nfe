package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	mathrand "math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/caian-org/nfe/internal/abrasf"
	"github.com/caian-org/nfe/internal/config"
	"github.com/caian-org/nfe/internal/nota"
	"github.com/caian-org/nfe/internal/service"
	"github.com/caian-org/nfe/internal/sigcorp"
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
	cfg.Nacional = nil // ABRASF/SOAP fixtures
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

	doc := etree.NewDocument()
	require.NoError(t, doc.ReadFromBytes(fake.lastBody))
	infEl := doc.FindElement("//InfDeclaracaoPrestacaoServico")
	require.NotNil(t, infEl)
	assert.Nil(t, infEl.SelectElement("Signature"), "emit signature must not be nested inside InfDeclaracaoPrestacaoServico")
	rpsEl := infEl.Parent()
	require.NotNil(t, rpsEl)
	sigEl := rpsEl.SelectElement("Signature")
	require.NotNil(t, sigEl, "emit signature must be a child of the Rps container")
	children := rpsEl.ChildElements()
	require.Len(t, children, 2)
	assert.Equal(t, infEl, children[0])
	assert.Equal(t, sigEl, children[1])
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

func TestQueryByRPS(t *testing.T) {
	svc, fake := fixtureService(t, false)
	fake.response = wrapSOAPResponse(`<ConsultarNfsePorRpsResposta><CompNfse><Nfse><InfNfse><Numero>1</Numero></InfNfse></Nfse></CompNfse></ConsultarNfsePorRpsResposta>`)

	res, err := svc.QueryByRPS(context.Background(), abrasf.IdentificacaoRps{
		Numero: 7,
		Serie:  "00000",
		Tipo:   abrasf.TipoRPS,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "ConsultarNfsePorRps", fake.lastAction)
	assert.Contains(t, string(fake.lastBody), "<ConsultarNfseRpsEnvio")
	assert.Contains(t, string(fake.lastBody), "<Numero>7</Numero>")
}

func TestQueryUsesDirectEndpointWithoutFetchingWSDL(t *testing.T) {
	var gotAction string
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			t.Fatalf("WSDL must not be fetched when endpoint is configured")
		}
		gotAction = r.Header.Get("SOAPAction")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		w.Write(wrapSOAPResponse(`<ConsultarNfseResposta><ListaNfse><CompNfse><Nfse><InfNfse><Numero>1</Numero></InfNfse></Nfse></CompNfse></ListaNfse></ConsultarNfseResposta>`))
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.Nacional = nil
	cfg.Autenticacao.Usuario = "u"
	cfg.Autenticacao.Senha = "p"
	cfg.SOAP.WSDLHomologacao = "http://127.0.0.1:1/unavailable?wsdl"
	cfg.SOAP.EndpointHomologacao = srv.URL
	cfg.SOAP.NamespaceHomologacao = "urn:test"
	cfg.SOAP.SOAPActionPrefix = "custom#"

	svc, err := service.New(service.Options{Config: cfg})
	require.NoError(t, err)

	res, err := svc.Query(context.Background(), abrasf.ConsultaQuery{Numero: "42"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, `"custom#ConsultarNfseServicoPrestado"`, gotAction)
	assert.Contains(t, gotBody, `xmlns:tns="urn:test"`)
	assert.Contains(t, gotBody, "<tns:ConsultarNfseServicoPrestadoRequest>")
	assert.NotContains(t, strings.ToLower(gotBody), "wsdl")
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

func TestQueryHTTPErrorSurfacesSOAPFault(t *testing.T) {
	svc, fake := fixtureService(t, false)
	fake.err = errors.New("soap: servidor retornou HTTP 500")
	fake.response = []byte(`<?xml version="1.0"?><soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><soap:Fault><faultstring>Falha interna do provedor</faultstring></soap:Fault></soap:Body></soap:Envelope>`)

	res, err := svc.Query(context.Background(), abrasf.ConsultaQuery{Numero: "1"})
	require.Error(t, err)
	require.NotNil(t, res)
	var me *service.MessagesError
	require.ErrorAs(t, err, &me)
	assert.Equal(t, "query", me.Action)
	require.Len(t, res.Mensagens, 1)
	assert.Equal(t, "SOAP", res.Mensagens[0].Codigo)
	assert.Equal(t, "Falha interna do provedor", res.Mensagens[0].Mensagem)
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

type fakeREST struct {
	recepcao     *sigcorp.Recepcao
	recepcaoErr  error
	recepcaoRaw  []byte
	consultas    []*sigcorp.Consulta
	consultaErr  error
	failFirst    int // number of initial Consultar calls that return consultaErr
	lastNNFSe    string
	lastDPS      []byte
	consultaHits int
}

func (f *fakeREST) Recepcionar(ctx context.Context, nNFSe string, dps []byte) (*sigcorp.Recepcao, []byte, error) {
	f.lastNNFSe = nNFSe
	f.lastDPS = dps
	return f.recepcao, f.recepcaoRaw, f.recepcaoErr
}

func (f *fakeREST) Consultar(ctx context.Context, protocolo string) (*sigcorp.Consulta, []byte, error) {
	if f.consultaErr != nil && (f.failFirst == 0 || f.consultaHits < f.failFirst) {
		f.consultaHits++
		return nil, nil, f.consultaErr
	}
	idx := f.consultaHits - f.failFirst
	if idx < 0 {
		idx = 0
	}
	if idx >= len(f.consultas) {
		idx = len(f.consultas) - 1
	}
	f.consultaHits++
	c := f.consultas[idx]
	return c, c.Raw, nil
}

func nacionalInput() *nota.Input {
	in := fixtureInput()
	in.CNBS = "115022000"
	in.Servico.CodigoTributacaoNacional = "010101"
	return in
}

func fixtureNacional(t *testing.T, withSigner bool) (*service.Service, *fakeREST) {
	t.Helper()
	cfg := config.Default()
	cfg.Prestador.CNPJ = "11222333000181"
	cfg.Prestador.InscricaoMunicipal = "123456"
	cfg.Configuracoes.CodigoMunicipio = "3516309"
	cfg.Configuracoes.ProximoNumeroRPS = 7
	cfg.Configuracoes.ProximoNumeroNFSe = 3
	rest := &fakeREST{}
	opts := service.Options{
		Config: cfg,
		REST:   rest,
		Now:    func() time.Time { return time.Date(2026, 10, 15, 9, 0, 0, 0, time.FixedZone("BRT", -3*3600)) },
	}
	if withSigner {
		cert, key := makeCert(t)
		signer, err := xmlsig.NewSigner(cert, key)
		require.NoError(t, err)
		opts.Signer = signer
	}
	svc, err := service.New(opts)
	require.NoError(t, err)
	return svc, rest
}

func TestNacionalEmitAcceptedBumpsBothCounters(t *testing.T) {
	svc, rest := fixtureNacional(t, true)
	rest.recepcao = &sigcorp.Recepcao{Protocolo: "p-1"}
	rest.recepcaoRaw = []byte(`{"protocolo":"p-1"}`)

	res, err := svc.Emit(context.Background(), nacionalInput())
	require.NoError(t, err)
	assert.Equal(t, "p-1", res.Protocolo)
	assert.Equal(t, 7, res.NumeroRPS)
	assert.Equal(t, "3", res.NumeroNFSe)
	assert.Equal(t, "3", rest.lastNNFSe)
	assert.Equal(t, 8, svc.Config().Configuracoes.ProximoNumeroRPS)
	assert.Equal(t, 4, svc.Config().Configuracoes.ProximoNumeroNFSe)

	doc := etree.NewDocument()
	require.NoError(t, doc.ReadFromBytes(rest.lastDPS))
	root := doc.Root()
	assert.Equal(t, "DPS", root.Tag)
	require.Len(t, root.ChildElements(), 2, "infDPS + Signature")
	assert.Equal(t, "Signature", root.ChildElements()[1].Tag)
	assert.Equal(t, "#"+root.ChildElements()[0].SelectAttrValue("Id", ""), root.FindElement(".//Reference").SelectAttrValue("URI", ""))
}

func TestNacionalEmitSkipsSignatureWhenDisabled(t *testing.T) {
	svc, rest := fixtureNacional(t, true)
	off := false
	svc.Config().Nacional.AssinarDPS = &off
	rest.recepcao = &sigcorp.Recepcao{Protocolo: "p"}
	_, err := svc.Emit(context.Background(), nacionalInput())
	require.NoError(t, err)
	assert.NotContains(t, string(rest.lastDPS), "<Signature")
}

func TestNacionalEmitHTTPErrorIsMessagesErrorAndKeepsCounters(t *testing.T) {
	svc, rest := fixtureNacional(t, false)
	rest.recepcaoErr = &sigcorp.HTTPError{URL: "u", Status: 403, Body: []byte(`{"message":"IM divergente"}`)}
	rest.recepcaoRaw = []byte(`{"message":"IM divergente"}`)

	res, err := svc.Emit(context.Background(), nacionalInput())
	require.Error(t, err)
	var me *service.MessagesError
	require.ErrorAs(t, err, &me)
	require.NotNil(t, res)
	assert.Contains(t, res.Mensagens[0].Mensagem, "HTTP 403")
	assert.Equal(t, 7, svc.Config().Configuracoes.ProximoNumeroRPS)
	assert.Equal(t, 3, svc.Config().Configuracoes.ProximoNumeroNFSe)
}

func TestNacionalEmitUnavailableIsPlainError(t *testing.T) {
	svc, rest := fixtureNacional(t, false)
	rest.recepcaoErr = &sigcorp.UnavailableError{URL: "https://x", Status: 502}
	_, err := svc.Emit(context.Background(), nacionalInput())
	require.Error(t, err)
	var me *service.MessagesError
	assert.False(t, errors.As(err, &me))
	assert.Contains(t, err.Error(), "indisponível")
}

func TestNacionalWaitConfirmacaoApproved(t *testing.T) {
	svc, rest := fixtureNacional(t, false)
	rest.consultas = []*sigcorp.Consulta{
		{Status: sigcorp.StatusEmProcessamento, Raw: []byte(`{"status":"em_processamento"}`)},
		{Status: sigcorp.StatusAprovado, NFSeXML: `<NFSe><infNFSe Id="NFS123"><nNFSe>3</nNFSe></infNFSe></NFSe>`, LinkImpressao: "https://l", Raw: []byte(`{"status":"aprovado"}`)},
	}
	res, err := svc.WaitConfirmacao(context.Background(), "p", time.Second, time.Millisecond)
	require.NoError(t, err)
	assert.True(t, res.Final)
	assert.Equal(t, sigcorp.StatusAprovado, res.Status)
	require.NotNil(t, res.NFSe)
	assert.Equal(t, "3", res.NFSe.Numero)
	assert.Equal(t, "123", res.NFSe.ChaveAcesso)
	assert.Equal(t, "https://l", res.LinkImpressao)
	assert.Equal(t, 2, rest.consultaHits)
}

func TestNacionalWaitConfirmacaoRejected(t *testing.T) {
	svc, rest := fixtureNacional(t, false)
	rest.consultas = []*sigcorp.Consulta{{Status: sigcorp.StatusRejeitado, Motivos: []string{"cNBS ausente"}, Raw: []byte(`{}`)}}
	res, err := svc.WaitConfirmacao(context.Background(), "p", time.Second, time.Millisecond)
	require.Error(t, err)
	var me *service.MessagesError
	require.ErrorAs(t, err, &me)
	require.NotNil(t, res)
	assert.True(t, res.Final)
	assert.Equal(t, "cNBS ausente", res.Mensagens[0].Mensagem)
}

func TestNacionalWaitConfirmacaoTimeoutIsPending(t *testing.T) {
	svc, rest := fixtureNacional(t, false)
	rest.consultas = []*sigcorp.Consulta{{Status: sigcorp.StatusEmProcessamento, Raw: []byte(`{}`)}}
	res, err := svc.WaitConfirmacao(context.Background(), "p", 30*time.Millisecond, 5*time.Millisecond)
	require.NoError(t, err)
	assert.False(t, res.Final)
	assert.Equal(t, sigcorp.StatusEmProcessamento, res.Status)
}

func TestNacionalWaitConfirmacaoToleratesTransientUnavailability(t *testing.T) {
	svc, rest := fixtureNacional(t, false)
	rest.consultaErr = &sigcorp.UnavailableError{URL: "u", Status: 503}
	rest.failFirst = 2
	rest.consultas = []*sigcorp.Consulta{{Status: sigcorp.StatusAprovado, Raw: []byte(`{}`)}}
	res, err := svc.WaitConfirmacao(context.Background(), "p", time.Second, time.Millisecond)
	require.NoError(t, err)
	assert.True(t, res.Final)
	assert.Equal(t, 3, rest.consultaHits)
}

func TestNacionalWaitConfirmacaoStopsOnHardError(t *testing.T) {
	svc, rest := fixtureNacional(t, false)
	rest.consultaErr = &sigcorp.HTTPError{URL: "u", Status: 401}
	_, err := svc.WaitConfirmacao(context.Background(), "p", time.Second, time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 401")
	assert.Equal(t, 1, rest.consultaHits)
}

func TestNacionalQueryAndCancelGuards(t *testing.T) {
	svc, _ := fixtureNacional(t, false)
	_, err := svc.Query(context.Background(), abrasf.ConsultaQuery{Numero: "1"})
	require.ErrorIs(t, err, service.ErrNaoSuportadoNacional)
	_, err = svc.QueryByRPS(context.Background(), abrasf.IdentificacaoRps{Numero: 1})
	require.ErrorIs(t, err, service.ErrNaoSuportadoNacional)
	_, err = svc.Cancel(context.Background(), "1", 1)
	require.ErrorIs(t, err, service.ErrNaoSuportadoNacional)
}

func TestNacionalDryRunDoesNotTouchNetworkOrCounters(t *testing.T) {
	svc, rest := fixtureNacional(t, false)
	res, err := svc.EmitDryRun(nacionalInput())
	require.NoError(t, err)
	assert.Equal(t, "3", res.NumeroNFSe)
	assert.Contains(t, string(res.SignedXML), "<DPS ")
	assert.Nil(t, rest.lastDPS)
	assert.Equal(t, 3, svc.Config().Configuracoes.ProximoNumeroNFSe)
}
