package cli_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/caian-org/nfe/internal/config"
)

// fakeSIGCORP emulates the WS Padrão Nacional: POST /v1/nfse/recepcao answers
// recepcaoStatus/recepcaoBody; GET /v1/nfse/consulta/{p} walks consultas in
// order, repeating the last one.
type fakeSIGCORP struct {
	srv            *httptest.Server
	mu             sync.Mutex
	recepcaoStatus int
	recepcaoBody   string
	consultas      []string
	posts          []*http.Request
	postBodies     []string
	gets           int
}

func newFakeSIGCORP(t *testing.T, recepcaoStatus int, recepcaoBody string, consultas ...string) *fakeSIGCORP {
	t.Helper()
	f := &fakeSIGCORP{recepcaoStatus: recepcaoStatus, recepcaoBody: recepcaoBody, consultas: consultas}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/nfse/recepcao":
			body, _ := io.ReadAll(r.Body)
			f.posts = append(f.posts, r)
			f.postBodies = append(f.postBodies, string(body))
			w.WriteHeader(f.recepcaoStatus)
			_, _ = w.Write([]byte(f.recepcaoBody))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/nfse/consulta/"):
			idx := f.gets
			if idx >= len(f.consultas) {
				idx = len(f.consultas) - 1
			}
			f.gets++
			_, _ = w.Write([]byte(f.consultas[idx]))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Cannot ` + r.Method + ` ` + r.URL.Path + `"}`))
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func nacionalWorkspace(t *testing.T, f *fakeSIGCORP) (workspace, cfgPath string) {
	t.Helper()
	cfg := config.Default()
	cfg.Prestador.CNPJ = "11222333000181"
	cfg.Prestador.InscricaoMunicipal = "123456"
	cfg.Configuracoes.CodigoMunicipio = "3516309"
	cfg.Configuracoes.ProximoNumeroRPS = 54
	cfg.Configuracoes.ProximoNumeroNFSe = 12
	cfg.Configuracoes.ConfirmTimeout = "2s"
	cfg.Configuracoes.ConfirmInterval = "10ms"
	cfg.Autenticacao.Usuario = ""
	cfg.Autenticacao.Senha = "segredo"
	cfg.Nacional.URLHomologacao = f.srv.URL
	cfg.Nacional.URLProducao = f.srv.URL
	workspace = t.TempDir()
	cfgPath = filepath.Join(workspace, "config.toml")
	require.NoError(t, config.Save(cfgPath, cfg))
	return workspace, cfgPath
}

func nacionalNota(t *testing.T, workspace, id string) string {
	t.Helper()
	body := `
cnbs = "115022000"
observacoes = "Projeto X"

[tomador]
cnpj = "44555666000170"
razao_social = "Tomador Test"

[tomador.endereco]
endereco = "R"
numero = "1"
bairro = "B"
codigo_municipio = "3550308"
municipio = "SAO PAULO"
uf = "SP"
cep = "01000000"

[servico]
discriminacao = "Teste"
valor_servicos = 3230.0
item_lista_servico = "0101"
codigo_tributacao_nacional = "010101"
aliquota = 2.0
`
	notasPath := filepath.Join(workspace, "notas")
	require.NoError(t, os.MkdirAll(notasPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(notasPath, id+".toml"), []byte(body), 0o644))
	return id
}

const nfseAprovadaXML = `<NFSe xmlns=\"http://www.sped.fazenda.gov.br/nfse\" versao=\"1.01\"><infNFSe Id=\"NFS35163092211222333000181000000000012202610123456789\"><nNFSe>12</nNFSe><dhProc>2026-10-15T09:31:00-03:00</dhProc><DPS><infDPS Id=\"x\"><nDPS>54</nDPS><toma><xNome>Tomador Test</xNome></toma><valores><vServPrest><vServ>3230.00</vServ></vServPrest></valores></infDPS></DPS></infNFSe></NFSe>`

func TestNacionalEmitApproved(t *testing.T) {
	f := newFakeSIGCORP(t, http.StatusAccepted, `{"protocolo":"prot-1"}`,
		`{"status":"em_processamento","protocolo":"prot-1"}`,
		`{"status":"aprovado","protocolo":"prot-1","nfse":"`+nfseAprovadaXML+`","linkImpressao":"https://x/danfse/1"}`,
	)
	workspace, cfgPath := nacionalWorkspace(t, f)
	nota := nacionalNota(t, workspace, "exemplo")

	out, err := runCmd(t, "-w", workspace, "emit", nota)
	require.NoError(t, err, out)
	assert.Contains(t, out, "NFS-e emitida")
	assert.Contains(t, out, "prot-1")
	assert.Contains(t, out, "35163092211222333000181000000000012202610123456789")
	assert.Contains(t, out, "https://x/danfse/1")
	assert.Contains(t, out, "12") // NFS-e number

	require.Len(t, f.posts, 1)
	req := f.posts[0]
	assert.Equal(t, "12", req.Header.Get("X-NFSe-nNFSe"))
	assert.Equal(t, "11222333000181", req.Header.Get("X-NFSe-Prestador-CNPJ"))
	user, pass, ok := req.BasicAuth()
	require.True(t, ok)
	assert.Equal(t, "123456", user, "inscrição municipal is the Basic Auth user")
	assert.Equal(t, "segredo", pass)
	assert.Equal(t, "application/xml", req.Header.Get("Content-Type"))
	assert.Contains(t, f.postBodies[0], `<DPS xmlns="http://www.sped.fazenda.gov.br/nfse" versao="1.01">`)
	assert.Contains(t, f.postBodies[0], `<infDPS Id="DPS351630921122233300018100001000000000000054">`)
	assert.Contains(t, f.postBodies[0], `<nDPS>54</nDPS>`)
	assert.Contains(t, f.postBodies[0], `<cNBS>115022000</cNBS>`)
	assert.GreaterOrEqual(t, f.gets, 2)

	reloaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, 55, reloaded.Configuracoes.ProximoNumeroRPS)
	assert.Equal(t, 13, reloaded.Configuracoes.ProximoNumeroNFSe)
}

func TestNacionalEmitJSON(t *testing.T) {
	f := newFakeSIGCORP(t, http.StatusAccepted, `{"protocolo":"prot-2"}`,
		`{"status":"aprovado","protocolo":"prot-2","nfse":"`+nfseAprovadaXML+`","linkImpressao":"https://x/danfse/2"}`,
	)
	workspace, _ := nacionalWorkspace(t, f)
	nota := nacionalNota(t, workspace, "exemplo")

	stdout, stderr, err := runCmdSplit(t, "--json", "-w", workspace, "emit", nota)
	require.NoError(t, err)
	assert.Empty(t, stderr)
	var got struct {
		Emit struct {
			Sucesso       bool   `json:"sucesso"`
			Modo          string `json:"modo"`
			Protocolo     string `json:"protocolo"`
			StatusProc    string `json:"status_processamento"`
			NumeroNFSe    string `json:"numero_nfse"`
			ChaveAcesso   string `json:"chave_acesso"`
			LinkImpressao string `json:"link_impressao"`
			NFSe          struct {
				Numero string `json:"numero"`
			} `json:"nfse"`
		} `json:"emit"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.True(t, got.Emit.Sucesso)
	assert.Equal(t, "nacional", got.Emit.Modo)
	assert.Equal(t, "prot-2", got.Emit.Protocolo)
	assert.Equal(t, "aprovado", got.Emit.StatusProc)
	assert.Equal(t, "12", got.Emit.NumeroNFSe)
	assert.Equal(t, "12", got.Emit.NFSe.Numero)
	assert.Len(t, got.Emit.ChaveAcesso, 50)
	assert.Equal(t, "https://x/danfse/2", got.Emit.LinkImpressao)
}

func TestNacionalEmitRejectedAfterAcceptanceConsumesNumbers(t *testing.T) {
	f := newFakeSIGCORP(t, http.StatusAccepted, `{"protocolo":"prot-3"}`,
		`{"status":"rejeitado","protocolo":"prot-3","motivos":["Prestador não autorizado a emitir nota com o código de serviço informado."]}`,
	)
	workspace, cfgPath := nacionalWorkspace(t, f)
	nota := nacionalNota(t, workspace, "exemplo")

	out, err := runCmd(t, "-w", workspace, "emit", nota)
	require.NoError(t, err, "business rejections render and exit 0")
	assert.Contains(t, out, "falha na emissão")
	assert.Contains(t, out, "prot-3")
	assert.Contains(t, out, "rejeitado")
	assert.Contains(t, out, "Prestador não autorizado")
	assert.Contains(t, out, "consumido")

	reloaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, 55, reloaded.Configuracoes.ProximoNumeroRPS)
	assert.Equal(t, 13, reloaded.Configuracoes.ProximoNumeroNFSe)
}

func TestNacionalEmitUnauthorizedLeavesCountersAlone(t *testing.T) {
	f := newFakeSIGCORP(t, http.StatusUnauthorized, `{"message":"Header Authorization Basic ausente.","error":"Unauthorized","statusCode":401}`)
	workspace, cfgPath := nacionalWorkspace(t, f)
	nota := nacionalNota(t, workspace, "exemplo")

	out, err := runCmd(t, "-w", workspace, "emit", nota)
	require.NoError(t, err)
	assert.Contains(t, out, "falha na emissão")
	assert.Contains(t, out, "HTTP 401")
	assert.Contains(t, out, "credenciais recusadas")

	reloaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, 54, reloaded.Configuracoes.ProximoNumeroRPS)
	assert.Equal(t, 12, reloaded.Configuracoes.ProximoNumeroNFSe)
}

func TestNacionalEmitNoConfirmationWait(t *testing.T) {
	f := newFakeSIGCORP(t, http.StatusAccepted, `{"protocolo":"prot-4"}`, `{"status":"em_processamento"}`)
	workspace, cfgPath := nacionalWorkspace(t, f)
	nota := nacionalNota(t, workspace, "exemplo")

	out, err := runCmd(t, "-w", workspace, "emit", nota, "--no-confirmation-wait")
	require.NoError(t, err)
	assert.Contains(t, out, "solicitação de NFS-e enviada")
	assert.Contains(t, out, "nfe query --protocolo prot-4")
	assert.Equal(t, 0, f.gets)

	reloaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, 13, reloaded.Configuracoes.ProximoNumeroNFSe)
}

func TestNacionalEmitPendingOnTimeout(t *testing.T) {
	f := newFakeSIGCORP(t, http.StatusAccepted, `{"protocolo":"prot-5"}`, `{"status":"em_processamento"}`)
	workspace, _ := nacionalWorkspace(t, f)
	setConfirmationDurations(t, workspace, "50ms", "10ms")
	nota := nacionalNota(t, workspace, "exemplo")

	out, err := runCmd(t, "-w", workspace, "emit", nota)
	require.NoError(t, err)
	assert.Contains(t, out, "solicitação de NFS-e enviada")
	assert.Contains(t, out, "prot-5")
	assert.GreaterOrEqual(t, f.gets, 1)
}

func TestNacionalEmitDryRunShowsDPS(t *testing.T) {
	f := newFakeSIGCORP(t, http.StatusAccepted, `{"protocolo":"never"}`)
	workspace, cfgPath := nacionalWorkspace(t, f)
	nota := nacionalNota(t, workspace, "exemplo")

	out, err := runCmd(t, "-w", workspace, "emit", nota, "--dry-run", "--verbose")
	require.NoError(t, err)
	assert.Contains(t, out, "dry-run")
	assert.Contains(t, out, "<DPS xmlns=")
	assert.Contains(t, out, "NFS-e número")
	assert.Empty(t, f.posts)

	reloaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, 12, reloaded.Configuracoes.ProximoNumeroNFSe)
}

func TestNacionalEmitWithoutIBSCBSSection(t *testing.T) {
	f := newFakeSIGCORP(t, http.StatusAccepted, `{"protocolo":"p"}`, `{"status":"aprovado","nfse":"`+nfseAprovadaXML+`"}`)
	workspace, cfgPath := nacionalWorkspace(t, f)
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	cfg.IBSCBS = nil
	require.NoError(t, config.Save(cfgPath, cfg))
	nota := nacionalNota(t, workspace, "exemplo")

	_, err = runCmd(t, "-w", workspace, "emit", nota)
	require.NoError(t, err)
	require.Len(t, f.postBodies, 1)
	assert.NotContains(t, f.postBodies[0], "<IBSCBS>")
	assert.Contains(t, f.postBodies[0], "<cNBS>115022000</cNBS>")
}

func TestNacionalQueryProtocolo(t *testing.T) {
	f := newFakeSIGCORP(t, http.StatusAccepted, ``,
		`{"status":"aprovado","protocolo":"prot-9","nfse":"`+nfseAprovadaXML+`","linkImpressao":"https://x/danfse/9"}`,
	)
	workspace, _ := nacionalWorkspace(t, f)

	out, err := runCmd(t, "-w", workspace, "query", "--protocolo", "prot-9")
	require.NoError(t, err)
	assert.Contains(t, out, "consulta do protocolo")
	assert.Contains(t, out, "aprovado")
	assert.Contains(t, out, "12")
	assert.Contains(t, out, "https://x/danfse/9")

	stdout, _, err := runCmdSplit(t, "--json", "-w", workspace, "query", "-p", "prot-9")
	require.NoError(t, err)
	var got struct {
		Query struct {
			Sucesso    bool   `json:"sucesso"`
			Protocolo  string `json:"protocolo"`
			StatusProc string `json:"status_processamento"`
			NFSes      []struct {
				Numero      string `json:"numero"`
				ChaveAcesso string `json:"chave_acesso"`
			} `json:"nfses"`
		} `json:"query"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.True(t, got.Query.Sucesso)
	assert.Equal(t, "prot-9", got.Query.Protocolo)
	assert.Equal(t, "aprovado", got.Query.StatusProc)
	require.Len(t, got.Query.NFSes, 1)
	assert.Equal(t, "12", got.Query.NFSes[0].Numero)
}

func TestNacionalQueryProtocoloRejected(t *testing.T) {
	f := newFakeSIGCORP(t, http.StatusAccepted, ``, `{"status":"recusada","motivo":"E0037 município não conveniado"}`)
	workspace, _ := nacionalWorkspace(t, f)

	out, err := runCmd(t, "-w", workspace, "query", "--protocolo", "prot-x")
	require.NoError(t, err)
	assert.Contains(t, out, "falha na consulta")
	assert.Contains(t, out, "recusada")
	assert.Contains(t, out, "E0037")
}

func TestNacionalQueryProtocoloPending(t *testing.T) {
	f := newFakeSIGCORP(t, http.StatusAccepted, ``, `{"status":"em_processamento","protocolo":"prot-y"}`)
	workspace, _ := nacionalWorkspace(t, f)

	out, err := runCmd(t, "-w", workspace, "query", "--protocolo", "prot-y")
	require.NoError(t, err)
	assert.Contains(t, out, "em_processamento")
	assert.Contains(t, out, "ainda não disponível")
}

func TestNacionalRefusesABRASFOnlyOperations(t *testing.T) {
	f := newFakeSIGCORP(t, http.StatusAccepted, ``)
	workspace, _ := nacionalWorkspace(t, f)

	_, err := runCmd(t, "-w", workspace, "query", "--numero", "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--protocolo")

	_, err = runCmd(t, "-w", workspace, "cancel", "--numero", "1", "--codigo", "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelamento")

	_, err = runCmd(t, "-w", workspace, "query", "--protocolo", "p", "--numero", "1")
	require.Error(t, err)
}

func TestNacionalStatusShowsMode(t *testing.T) {
	f := newFakeSIGCORP(t, http.StatusAccepted, ``)
	workspace, _ := nacionalWorkspace(t, f)

	out, err := runCmd(t, "-w", workspace, "status")
	require.NoError(t, err)
	assert.Contains(t, out, "nacional")
	assert.Contains(t, out, f.srv.URL)
	assert.Contains(t, out, "próximo nº NFS-e")
	assert.Contains(t, out, "12")

	stdout, _, err := runCmdSplit(t, "--json", "-w", workspace, "status")
	require.NoError(t, err)
	var got struct {
		Status struct {
			Modo              string `json:"modo"`
			NacionalURL       string `json:"nacional_url"`
			ProximoNumeroNFSe int    `json:"proximo_numero_nfse"`
			UsingBasicAuth    bool   `json:"using_basic_auth"`
		} `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Equal(t, "nacional", got.Status.Modo)
	assert.Equal(t, f.srv.URL, got.Status.NacionalURL)
	assert.Equal(t, 12, got.Status.ProximoNumeroNFSe)
	assert.True(t, got.Status.UsingBasicAuth)
}

func TestNacionalEmitRejectsNotaWithoutCTribNac(t *testing.T) {
	f := newFakeSIGCORP(t, http.StatusAccepted, `{"protocolo":"never"}`)
	workspace, _ := nacionalWorkspace(t, f)
	nota := notaFile(t, workspace, "legacy") // ABRASF-style nota without the national fields

	_, err := runCmd(t, "-w", workspace, "emit", nota, "--dry-run")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "codigo_tributacao_nacional")
	assert.Empty(t, f.posts)
}
