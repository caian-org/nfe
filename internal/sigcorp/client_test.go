package sigcorp_test

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/caian-org/nfe/internal/sigcorp"
)

func newClient(t *testing.T, base string) *sigcorp.Client {
	t.Helper()
	c, err := sigcorp.NewClient(sigcorp.Options{BaseURL: base + "/", Usuario: "123456", Senha: "s3gredo", CNPJ: "11.222.333/0001-81"})
	require.NoError(t, err)
	return c
}

func TestRecepcionarSendsHeadersAndParsesProtocol(t *testing.T) {
	var got *http.Request
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"protocolo":"abc-123"}`))
	}))
	defer srv.Close()

	rec, raw, err := newClient(t, srv.URL).Recepcionar(context.Background(), "42", []byte("<DPS/>"))
	require.NoError(t, err)
	assert.Equal(t, "abc-123", rec.Protocolo)
	assert.JSONEq(t, `{"protocolo":"abc-123"}`, string(raw))

	assert.Equal(t, http.MethodPost, got.Method)
	assert.Equal(t, "/v1/nfse/recepcao", got.URL.Path)
	assert.Equal(t, "application/xml", got.Header.Get("Content-Type"))
	assert.Equal(t, "42", got.Header.Get("X-NFSe-nNFSe"))
	assert.Equal(t, "11222333000181", got.Header.Get("X-NFSe-Prestador-CNPJ"))
	assert.Equal(t, "Basic "+base64.StdEncoding.EncodeToString([]byte("123456:s3gredo")), got.Header.Get("Authorization"))
	assert.Equal(t, "<DPS/>", string(body))
}

func TestRecepcionarHTTPErrors(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusBadRequest, `{"message":"Header X-NFSe-nNFSe ausente."}`, "HTTP 400"},
		{http.StatusUnauthorized, `{"message":"Header Authorization Basic ausente.","error":"Unauthorized","statusCode":401}`, "credenciais recusadas"},
		{http.StatusForbidden, ``, "HTTP 403"},
		{http.StatusBadRequest, `{"codigo_erro":"ESTRUTURAL","mensagem_erro":"Tipo inválido para /toma/fone; esperado N."}`, "ESTRUTURAL: Tipo inválido para /toma/fone"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			_, raw, err := newClient(t, srv.URL).Recepcionar(context.Background(), "1", []byte("<DPS/>"))
			require.Error(t, err)
			var he *sigcorp.HTTPError
			require.ErrorAs(t, err, &he)
			assert.Equal(t, tc.status, he.Status)
			assert.Contains(t, err.Error(), tc.want)
			assert.Equal(t, tc.body, string(raw))
			if tc.status == http.StatusUnauthorized {
				assert.Contains(t, err.Error(), "Header Authorization Basic ausente.")
			}
		})
	}
}

func TestServerErrorIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	_, _, err := newClient(t, srv.URL).Consultar(context.Background(), "p")
	var ue *sigcorp.UnavailableError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, http.StatusBadGateway, ue.Status)
	assert.Contains(t, err.Error(), "indisponível")
}

func TestDNSFailureIsUnavailable(t *testing.T) {
	c, err := sigcorp.NewClient(sigcorp.Options{BaseURL: "https://nfe-nao-existe.invalid", Usuario: "u", Senha: "p", CNPJ: "1"})
	require.NoError(t, err)
	_, _, err = c.Consultar(context.Background(), "p")
	var ue *sigcorp.UnavailableError
	require.ErrorAs(t, err, &ue)
	assert.Contains(t, err.Error(), "nfe-nao-existe.invalid")
}

func TestConsultarStatuses(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		status string
		final  bool
		xml    string
		link   string
		reason string
	}{
		{"processing", `{"status":"em_processamento","protocolo":"p1"}`, sigcorp.StatusEmProcessamento, false, "", "", ""},
		{"approved", `{"status":"aprovado","protocolo":"p1","nfse":"<NFSe/>","linkImpressao":"https://x/danfse"}`, sigcorp.StatusAprovado, true, "<NFSe/>", "https://x/danfse", ""},
		{"rejected list of strings", `{"status":"rejeitado","motivos":["Prestador não autorizado","cNBS ausente"]}`, sigcorp.StatusRejeitado, true, "", "", "cNBS ausente"},
		{"rejected list of objects", `{"status":"rejeitado","erros":[{"codigo":"ALIQUOTA_SIMPLES_INVALIDA","mensagem":"pAliq obrigatório"}]}`, sigcorp.StatusRejeitado, true, "", "", "ALIQUOTA_SIMPLES_INVALIDA: pAliq obrigatório"},
		{"refused by ADN", `{"status":"recusada","motivo":"E0037 município não conveniado"}`, sigcorp.StatusRecusada, true, "", "", "E0037"},
		{"internal error", `{"status":"erro_interno","mensagem":"Falha técnica"}`, sigcorp.StatusErroInterno, true, "", "", "Falha técnica"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				assert.Equal(t, http.MethodGet, r.Method)
				assert.NotEmpty(t, r.Header.Get("Authorization"))
				assert.Equal(t, "11222333000181", r.Header.Get("X-NFSe-Prestador-CNPJ"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			cons, _, err := newClient(t, srv.URL).Consultar(context.Background(), "p1")
			require.NoError(t, err)
			assert.Equal(t, "/v1/nfse/consulta/p1", gotPath)
			assert.Equal(t, tc.status, cons.Status)
			assert.Equal(t, tc.final, cons.Final())
			assert.Equal(t, tc.xml, cons.NFSeXML)
			assert.Equal(t, tc.link, cons.LinkImpressao)
			if tc.reason != "" {
				require.NotEmpty(t, cons.Motivos)
				joined := ""
				for _, m := range cons.Motivos {
					joined += m + "\n"
				}
				assert.Contains(t, joined, tc.reason)
			}
		})
	}
}

func TestParseConsultaRequiresStatus(t *testing.T) {
	_, err := sigcorp.ParseConsulta([]byte(`{"protocolo":"x"}`))
	require.Error(t, err)
	_, err = sigcorp.ParseConsulta([]byte(`not json`))
	require.Error(t, err)
}

func TestNewClientValidation(t *testing.T) {
	_, err := sigcorp.NewClient(sigcorp.Options{})
	require.Error(t, err)
	_, err = sigcorp.NewClient(sigcorp.Options{BaseURL: "https://x", Usuario: "u", Senha: "p"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, err))
}
