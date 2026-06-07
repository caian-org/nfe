package cli_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/caian-org/nfe/internal/config"
)

// fakeABRASF returns an httptest server that responds to GET ?wsdl with a
// minimal WSDL pointing at itself, and to POST requests with the supplied
// response body. The WSDL declares the SOAP operations used by the CLI.
func fakeABRASF(t *testing.T, responseBody string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || strings.HasSuffix(r.URL.RequestURI(), "?wsdl") {
			w.Header().Set("Content-Type", "text/xml")
			w.Write([]byte(`<?xml version="1.0"?>
<definitions xmlns="http://schemas.xmlsoap.org/wsdl/" xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/" targetNamespace="` + srv.URL + `">
  <binding name="B" type="P">
    <operation name="GerarNfse"><soap:operation soapAction="nfs#GerarNfse"/></operation>
    <operation name="ConsultarNfseServicoPrestado"><soap:operation soapAction="nfs#ConsultarNfseServicoPrestado"/></operation>
    <operation name="CancelarNfse"><soap:operation soapAction="nfs#CancelarNfse"/></operation>
  </binding>
  <service name="S"><port name="P" binding="tns:B"><soap:address location="` + srv.URL + `/svc"/></port></service>
</definitions>`))
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		w.Write([]byte(responseBody))
	}))
	return srv
}

func fakeABRASFByAction(t *testing.T, responses map[string]string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || strings.HasSuffix(r.URL.RequestURI(), "?wsdl") {
			w.Header().Set("Content-Type", "text/xml")
			w.Write([]byte(`<?xml version="1.0"?>
<definitions xmlns="http://schemas.xmlsoap.org/wsdl/" xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/" targetNamespace="` + srv.URL + `">
  <binding name="B" type="P">
    <operation name="GerarNfse"><soap:operation soapAction="nfs#GerarNfse"/></operation>
    <operation name="ConsultarNfseServicoPrestado"><soap:operation soapAction="nfs#ConsultarNfseServicoPrestado"/></operation>
    <operation name="ConsultarNfsePorRps"><soap:operation soapAction="nfs#ConsultarNfsePorRps"/></operation>
    <operation name="CancelarNfse"><soap:operation soapAction="nfs#CancelarNfse"/></operation>
  </binding>
  <service name="S"><port name="P" binding="tns:B"><soap:address location="` + srv.URL + `/svc"/></port></service>
</definitions>`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		action := "default"
		for candidate := range responses {
			if strings.Contains(string(body), candidate) {
				action = candidate
				break
			}
		}
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		w.Write([]byte(responses[action]))
	}))
	return srv
}

// workspaceWith returns a workspace with a config that targets srv (a fake ABRASF
// server serving WSDL at GET and SOAP at POST).
func workspaceWith(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	cfg := config.Default()
	cfg.Autenticacao.Usuario = "u"
	cfg.Autenticacao.Senha = "p"
	cfg.SOAP.WSDLHomologacao = srv.URL + "?wsdl"
	cfg.SOAP.WSDLProducao = srv.URL + "?wsdl"
	workspace := t.TempDir()
	require.NoError(t, config.Save(filepath.Join(workspace, "config.toml"), cfg))
	return workspace
}

func setConfirmationDurations(t *testing.T, workspace, timeout, interval string) {
	t.Helper()
	cfgPath := filepath.Join(workspace, "config.toml")
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	cfg.Configuracoes.ConfirmTimeout = timeout
	cfg.Configuracoes.ConfirmInterval = interval
	require.NoError(t, config.Save(cfgPath, cfg))
}

func notaFile(t *testing.T, workspace, id string) string {
	t.Helper()
	body := `
[tomador]
cnpj = "44555666000170"
razao_social = "Tomador Test"

[tomador.endereco]
endereco = "R"
numero = "1"
bairro = "B"
codigo_municipio = "7654321"
uf = "SP"
cep = "01000000"

[servico]
discriminacao = "Teste"
valor_servicos = 100.0
item_lista_servico = "0101"
aliquota = 5.0
`
	notasPath := filepath.Join(workspace, "notas")
	require.NoError(t, os.MkdirAll(notasPath, 0o755))
	path := filepath.Join(notasPath, id+".toml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return id
}

func soapEnvelope(body string) string {
	return `<?xml version="1.0"?><soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>` + body + `</soap:Body></soap:Envelope>`
}

func TestEmitDryRunNoNetwork(t *testing.T) {
	// Point at a URL that would refuse if dialed.
	cfg := config.Default()
	cfg.SOAP.WSDLHomologacao = "http://127.0.0.1:1"
	cfg.SOAP.WSDLProducao = "http://127.0.0.1:1"
	cfg.Autenticacao.Usuario = "u"
	cfg.Autenticacao.Senha = "p"
	workspace := t.TempDir()
	cfgPath := filepath.Join(workspace, "config.toml")
	require.NoError(t, config.Save(cfgPath, cfg))
	notaID := notaFile(t, workspace, "nota")

	out, err := runCmd(t, "--workspace", workspace, "emit", notaID, "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out, "dry-run")
	assert.Contains(t, out, "RPS número")

	// Counter must NOT have been bumped.
	reloaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, 1, reloaded.Configuracoes.ProximoNumeroRPS)
}

func TestEmitDryRunProgressGoesToStderr(t *testing.T) {
	cfg := config.Default()
	cfg.SOAP.WSDLHomologacao = "http://127.0.0.1:1"
	cfg.SOAP.WSDLProducao = "http://127.0.0.1:1"
	cfg.Autenticacao.Usuario = "u"
	cfg.Autenticacao.Senha = "p"
	workspace := t.TempDir()
	require.NoError(t, config.Save(filepath.Join(workspace, "config.toml"), cfg))
	notaID := notaFile(t, workspace, "nota")

	stdout, stderr, err := runCmdSplit(t, "--workspace", workspace, "emit", notaID, "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, stdout, "dry-run")
	assert.Contains(t, stdout, "RPS número")
	assert.NotContains(t, stdout, "nfe emit ·")
	assert.Contains(t, stderr, "nfe emit · nota")
	assert.Contains(t, stderr, "xml")
	assert.Contains(t, stderr, "config")
}

func TestEmitDryRunJSONDoesNotRenderProgress(t *testing.T) {
	cfg := config.Default()
	cfg.SOAP.WSDLHomologacao = "http://127.0.0.1:1"
	cfg.SOAP.WSDLProducao = "http://127.0.0.1:1"
	cfg.Autenticacao.Usuario = "u"
	cfg.Autenticacao.Senha = "p"
	workspace := t.TempDir()
	require.NoError(t, config.Save(filepath.Join(workspace, "config.toml"), cfg))
	notaID := notaFile(t, workspace, "nota")

	stdout, stderr, err := runCmdSplit(t, "--json", "--workspace", workspace, "emit", notaID, "--dry-run")
	require.NoError(t, err)
	assert.Empty(t, stderr)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Equal(t, "emit", got["event"])
}

func TestEmitDryRunVerboseRendersSignedXML(t *testing.T) {
	cfg := config.Default()
	cfg.SOAP.WSDLHomologacao = "http://127.0.0.1:1"
	cfg.SOAP.WSDLProducao = "http://127.0.0.1:1"
	cfg.Autenticacao.Usuario = "u"
	cfg.Autenticacao.Senha = "p"
	workspace := t.TempDir()
	require.NoError(t, config.Save(filepath.Join(workspace, "config.toml"), cfg))
	notaID := notaFile(t, workspace, "nota")

	out, err := runCmd(t, "--workspace", workspace, "emit", notaID, "--dry-run", "--verbose")
	require.NoError(t, err)
	assert.Contains(t, out, "XML assinado:")
	assert.Contains(t, out, "<GerarNfseEnvio")
	assert.NotContains(t, out, "resposta SOAP:")
}

func TestEmitSuccessBumpsCounterOnDisk(t *testing.T) {
	srv := fakeABRASF(t, soapEnvelope(`<GerarNfseResposta xmlns="http://www.abrasf.org.br/nfse.xsd"><ListaNfse><CompNfse><Nfse><InfNfse><Numero>9999</Numero></InfNfse></Nfse></CompNfse></ListaNfse></GerarNfseResposta>`))
	defer srv.Close()

	workspace := workspaceWith(t, srv)
	notaID := notaFile(t, workspace, "mova")
	out, err := runCmd(t, "-w", workspace, "emit", notaID)
	require.NoError(t, err)
	assert.Contains(t, out, "NFS-e emitida")
	assert.Contains(t, out, "9999")

	reloaded, err := config.Load(filepath.Join(workspace, "config.toml"))
	require.NoError(t, err)
	assert.Equal(t, 2, reloaded.Configuracoes.ProximoNumeroRPS,
		"after a successful emit, the on-disk counter must be one higher")
}

func TestEmitVerboseRendersSOAPResponse(t *testing.T) {
	body := soapEnvelope(`<GerarNfseResposta xmlns="http://www.abrasf.org.br/nfse.xsd"><ListaNfse><CompNfse><Nfse><InfNfse><Numero>9999</Numero></InfNfse></Nfse></CompNfse></ListaNfse></GerarNfseResposta>`)
	srv := fakeABRASF(t, body)
	defer srv.Close()

	workspace := workspaceWith(t, srv)
	notaID := notaFile(t, workspace, "mova")
	out, err := runCmd(t, "-w", workspace, "emit", notaID, "--verbose")
	require.NoError(t, err)
	assert.Contains(t, out, "XML assinado:")
	assert.Contains(t, out, "resposta SOAP:")
	assert.Contains(t, out, "<GerarNfseResposta")
}

func TestEmitPollsRPSConfirmationWhenResponseIsAsync(t *testing.T) {
	srv := fakeABRASFByAction(t, map[string]string{
		"GerarNfseRequest":           soapEnvelope(`<GerarNfseResposta xmlns="http://www.abrasf.org.br/nfse.xsd"><Mensagem>Solicitação recebida! Aguarde a confirmação da Nota Fiscal pelo Sefaz/ADN.</Mensagem></GerarNfseResposta>`),
		"ConsultarNfsePorRpsRequest": soapEnvelope(`<ConsultarNfsePorRpsResposta xmlns="http://www.abrasf.org.br/nfse.xsd"><CompNfse><Nfse><InfNfse><Numero>8888</Numero><CodigoVerificacao>ABC123</CodigoVerificacao><DataEmissao>2026-06-07T10:00:00-03:00</DataEmissao><UrlVisualizacao>https://example.test/nfse/8888</UrlVisualizacao><DeclaracaoPrestacaoServico><InfDeclaracaoPrestacaoServico><Servico><Valores><ValorServicos>100.00</ValorServicos></Valores></Servico><TomadorServico><RazaoSocial>ACME</RazaoSocial></TomadorServico></InfDeclaracaoPrestacaoServico></DeclaracaoPrestacaoServico></InfNfse></Nfse></CompNfse></ConsultarNfsePorRpsResposta>`),
	})
	defer srv.Close()

	workspace := workspaceWith(t, srv)
	notaID := notaFile(t, workspace, "mova")
	out, err := runCmd(t, "-w", workspace, "emit", notaID, "--verbose")
	require.NoError(t, err)
	assert.Contains(t, out, "NFS-e emitida")
	assert.Contains(t, out, "8888")
	assert.Contains(t, out, "ABC123")
	assert.Contains(t, out, "https://example.test/nfse/8888")
	assert.Contains(t, out, "resposta consulta RPS:")
}

func TestEmitKeepsPendingWhenRPSConfirmationHasOnlySituacao(t *testing.T) {
	srv := fakeABRASFByAction(t, map[string]string{
		"GerarNfseRequest":           soapEnvelope(`<GerarNfseResposta xmlns="http://www.abrasf.org.br/nfse.xsd"><Mensagem>Solicitação recebida! Aguarde a confirmação da Nota Fiscal pelo Sefaz/ADN.</Mensagem></GerarNfseResposta>`),
		"ConsultarNfsePorRpsRequest": soapEnvelope(`<ConsultarNfseRpsResposta xmlns="http://www.abrasf.org.br/nfse.xsd"><CompNfse><Situacao><Rps><Numero>7</Numero><Serie>00000</Serie><Tipo>1</Tipo></Rps><Status>0</Status><Mensagem>Aguardando envio para o ADN</Mensagem></Situacao></CompNfse></ConsultarNfseRpsResposta>`),
	})
	defer srv.Close()

	workspace := workspaceWith(t, srv)
	setConfirmationDurations(t, workspace, "20ms", "1ms")
	notaID := notaFile(t, workspace, "mova")
	out, err := runCmd(t, "-w", workspace, "emit", notaID)
	require.NoError(t, err)
	assert.Contains(t, out, "solicitação de NFS-e enviada")
	assert.Contains(t, out, "solicitação recebida; confirmação da NFS-e ainda pendente")
	assert.NotContains(t, out, "NFS-e número")
}

func TestEmitNoConfirmationWaitSkipsRPSPolling(t *testing.T) {
	srv := fakeABRASFByAction(t, map[string]string{
		"GerarNfseRequest":           soapEnvelope(`<GerarNfseResposta xmlns="http://www.abrasf.org.br/nfse.xsd"><Mensagem>Solicitação recebida! Aguarde a confirmação da Nota Fiscal pelo Sefaz/ADN.</Mensagem></GerarNfseResposta>`),
		"ConsultarNfsePorRpsRequest": soapEnvelope(`<ConsultarNfsePorRpsResposta xmlns="http://www.abrasf.org.br/nfse.xsd"><CompNfse><Nfse><InfNfse><Numero>8888</Numero></InfNfse></Nfse></CompNfse></ConsultarNfsePorRpsResposta>`),
	})
	defer srv.Close()

	workspace := workspaceWith(t, srv)
	notaID := notaFile(t, workspace, "mova")
	out, err := runCmd(t, "-w", workspace, "emit", notaID, "--no-confirmation-wait")
	require.NoError(t, err)
	assert.Contains(t, out, "solicitação recebida; confirmação da NFS-e ainda pendente")
	assert.NotContains(t, out, "8888")
}

func TestEmitErrorsLeaveCounterAlone(t *testing.T) {
	srv := fakeABRASF(t, soapEnvelope(`<GerarNfseResposta><ListaMensagemRetorno><MensagemRetorno><Codigo>E1</Codigo><Mensagem>rejeitado</Mensagem></MensagemRetorno></ListaMensagemRetorno></GerarNfseResposta>`))
	defer srv.Close()

	workspace := workspaceWith(t, srv)
	notaID := notaFile(t, workspace, "mova")
	out, err := runCmd(t, "-w", workspace, "emit", notaID)
	require.NoError(t, err, "WS rejection must NOT surface as a CLI error; the renderer handles it")
	assert.Contains(t, out, "falha na emissão")
	assert.Contains(t, out, "rejeitado")

	reloaded, err := config.Load(filepath.Join(workspace, "config.toml"))
	require.NoError(t, err)
	assert.Equal(t, 1, reloaded.Configuracoes.ProximoNumeroRPS)
}

func TestQueryByNumeroRendersTable(t *testing.T) {
	// Real-world ABRASF responses put Servico/TomadorServico inside
	// DeclaracaoPrestacaoServico/InfDeclaracaoPrestacaoServico, not directly
	// inside InfNfse.
	srv := fakeABRASF(t, soapEnvelope(`<ConsultarNfseResposta><ListaNfse><CompNfse><Nfse><InfNfse><Numero>42</Numero><CodigoVerificacao>ABC</CodigoVerificacao><DataEmissao>2026-05-01T10:00:00-03:00</DataEmissao><ValoresNfse><BaseCalculo>1500.00</BaseCalculo></ValoresNfse><DeclaracaoPrestacaoServico><InfDeclaracaoPrestacaoServico><Servico><Valores><ValorServicos>1500.00</ValorServicos></Valores></Servico><TomadorServico><RazaoSocial>ACME</RazaoSocial></TomadorServico></InfDeclaracaoPrestacaoServico></DeclaracaoPrestacaoServico></InfNfse></Nfse></CompNfse></ListaNfse></ConsultarNfseResposta>`))
	defer srv.Close()

	workspace := workspaceWith(t, srv)
	out, err := runCmd(t, "-w", workspace, "query", "-n", "42")
	require.NoError(t, err)
	assert.Contains(t, out, "42")
	assert.Contains(t, out, "ABC")
	assert.Contains(t, out, "ACME")
}

func TestQueryRequiresFilter(t *testing.T) {
	srv := fakeABRASF(t, `<should-not-be-called/>`)
	defer srv.Close()

	workspace := workspaceWith(t, srv)
	_, err := runCmd(t, "-w", workspace, "query")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--numero ou ambos --data-inicial")
}

func TestQueryJSON(t *testing.T) {
	srv := fakeABRASF(t, soapEnvelope(`<r><ListaNfse><CompNfse><Nfse><InfNfse><Numero>1</Numero></InfNfse></Nfse></CompNfse></ListaNfse></r>`))
	defer srv.Close()

	workspace := workspaceWith(t, srv)
	out, err := runCmd(t, "--json", "-w", workspace, "query", "-n", "1")
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	q := got["query"].(map[string]any)
	assert.True(t, q["sucesso"].(bool))
	nfses := q["nfses"].([]any)
	require.Len(t, nfses, 1)
}

func TestCancelSuccess(t *testing.T) {
	srv := fakeABRASF(t, soapEnvelope(`<CancelarNfseResposta><Confirmacao/></CancelarNfseResposta>`))
	defer srv.Close()

	workspace := workspaceWith(t, srv)
	out, err := runCmd(t, "-w", workspace, "cancel", "-n", "100", "--codigo", "1")
	require.NoError(t, err)
	assert.Contains(t, out, "NFS-e cancelada")
	assert.Contains(t, out, "100")
}

func TestCancelRejectsBadCodigo(t *testing.T) {
	srv := fakeABRASF(t, `<should-not-be-called/>`)
	defer srv.Close()

	workspace := workspaceWith(t, srv)
	_, err := runCmd(t, "-w", workspace, "cancel", "-n", "100", "--codigo", "99")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "codigo")
}

func TestCancelMessagesSurfaced(t *testing.T) {
	srv := fakeABRASF(t, soapEnvelope(`<CancelarNfseResposta><ListaMensagemRetorno><MensagemRetorno><Codigo>X1</Codigo><Mensagem>nope</Mensagem></MensagemRetorno></ListaMensagemRetorno></CancelarNfseResposta>`))
	defer srv.Close()

	workspace := workspaceWith(t, srv)
	out, err := runCmd(t, "-w", workspace, "cancel", "-n", "100", "--codigo", "2")
	require.NoError(t, err)
	assert.Contains(t, out, "falha no cancelamento")
	assert.Contains(t, out, "nope")
}
