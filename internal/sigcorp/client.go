// Package sigcorp talks to the SIGCORP "WS Padrão Nacional" REST API used by
// the municipalities on the MeuMunicipio platform (e.g. Franco da Rocha).
// The API receives the national DPS XML and returns a protocol that is later
// polled for the resulting NFS-e.
package sigcorp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Status values returned by the consulta endpoint.
const (
	StatusEmProcessamento = "em_processamento"
	StatusRejeitado       = "rejeitado"
	StatusRecusada        = "recusada"
	StatusErroInterno     = "erro_interno"
	StatusAprovado        = "aprovado"
)

// Options configures a Client.
type Options struct {
	BaseURL string // e.g. https://webservice.meumunicipio.online
	Usuario string // inscrição municipal do prestador
	Senha   string
	CNPJ    string // prestador CNPJ, digits only
	Timeout time.Duration
	// HTTPClient overrides the default client (tests).
	HTTPClient *http.Client
}

// Client is a thin HTTP wrapper around the two SIGCORP endpoints.
type Client struct {
	base    string
	usuario string
	senha   string
	cnpj    string
	http    *http.Client
}

// NewClient validates the options and returns a Client.
func NewClient(opts Options) (*Client, error) {
	if opts.BaseURL == "" {
		return nil, errors.New("sigcorp: URL base é obrigatória")
	}
	if opts.Usuario == "" || opts.Senha == "" {
		return nil, errors.New("sigcorp: usuário (inscrição municipal) e senha são obrigatórios")
	}
	if opts.CNPJ == "" {
		return nil, errors.New("sigcorp: CNPJ do prestador é obrigatório")
	}
	hc := opts.HTTPClient
	if hc == nil {
		timeout := opts.Timeout
		if timeout == 0 {
			timeout = 60 * time.Second
		}
		hc = &http.Client{Timeout: timeout}
	}
	return &Client{
		base:    strings.TrimRight(opts.BaseURL, "/"),
		usuario: opts.Usuario,
		senha:   opts.Senha,
		cnpj:    digitsOnly(opts.CNPJ),
		http:    hc,
	}, nil
}

// Recepcao is the body of a successful (202) recepção.
type Recepcao struct {
	Protocolo string          `json:"protocolo"`
	Raw       json.RawMessage `json:"-"`
}

// Consulta is the body returned by the consulta endpoint. Motivos gathers the
// rejection reasons regardless of the exact key the platform uses.
type Consulta struct {
	Status        string
	Protocolo     string
	NFSeXML       string
	LinkImpressao string
	Motivos       []string
	Raw           json.RawMessage
}

// Final reports whether the status will not change with further polling.
func (c *Consulta) Final() bool {
	return c != nil && c.Status != "" && c.Status != StatusEmProcessamento
}

// Recepcionar posts a DPS. nNFSe is the NFS-e number chosen by the prestador.
func (c *Client) Recepcionar(ctx context.Context, nNFSe string, dps []byte) (*Recepcao, []byte, error) {
	url := c.base + "/v1/nfse/recepcao"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(dps))
	if err != nil {
		return nil, nil, fmt.Errorf("sigcorp: construir requisição: %w", err)
	}
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-NFSe-nNFSe", nNFSe)
	c.auth(req)

	status, body, err := c.do(req)
	if err != nil {
		return nil, body, err
	}
	if status != http.StatusAccepted && status != http.StatusOK {
		return nil, body, httpError(url, status, body)
	}
	var rec Recepcao
	if err := json.Unmarshal(body, &rec); err != nil {
		return nil, body, fmt.Errorf("sigcorp: resposta da recepção não é JSON válido: %w", err)
	}
	if rec.Protocolo == "" {
		return nil, body, errors.New("sigcorp: recepção aceita sem protocolo na resposta")
	}
	rec.Raw = append(json.RawMessage(nil), body...)
	return &rec, body, nil
}

// Consultar fetches the processing status of a protocol.
func (c *Client) Consultar(ctx context.Context, protocolo string) (*Consulta, []byte, error) {
	if protocolo == "" {
		return nil, nil, errors.New("sigcorp: protocolo é obrigatório")
	}
	url := c.base + "/v1/nfse/consulta/" + protocolo
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("sigcorp: construir requisição: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	c.auth(req)

	status, body, err := c.do(req)
	if err != nil {
		return nil, body, err
	}
	if status != http.StatusOK {
		return nil, body, httpError(url, status, body)
	}
	cons, err := ParseConsulta(body)
	if err != nil {
		return nil, body, err
	}
	return cons, body, nil
}

func (c *Client) auth(req *http.Request) {
	req.SetBasicAuth(c.usuario, c.senha)
	req.Header.Set("X-NFSe-Prestador-CNPJ", c.cnpj)
}

func (c *Client) do(req *http.Request) (int, []byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, classifyTransportErr(req.URL.String(), err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("sigcorp: ler resposta: %w", err)
	}
	if resp.StatusCode >= 500 {
		return resp.StatusCode, body, &UnavailableError{URL: req.URL.String(), Status: resp.StatusCode}
	}
	return resp.StatusCode, body, nil
}

// ParseConsulta decodes a consulta body tolerantly: the manual fixes the
// status values but not the shape of the rejection reasons.
func ParseConsulta(body []byte) (*Consulta, error) {
	var generic map[string]any
	if err := json.Unmarshal(body, &generic); err != nil {
		return nil, fmt.Errorf("sigcorp: resposta da consulta não é JSON válido: %w", err)
	}
	cons := &Consulta{Raw: append(json.RawMessage(nil), body...)}
	cons.Status = strings.ToLower(strings.TrimSpace(str(generic["status"])))
	cons.Protocolo = str(generic["protocolo"])
	cons.NFSeXML = str(generic["nfse"])
	cons.LinkImpressao = firstStr(generic, "linkImpressao", "link_impressao", "linkDanfse")
	for _, key := range []string{"motivos", "motivo", "erros", "erro", "mensagens", "mensagem", "detalhes", "detalhe", "message", "errors"} {
		if v, ok := generic[key]; ok {
			cons.Motivos = append(cons.Motivos, flatten(v)...)
		}
	}
	if code, msg := str(generic["codigo_erro"]), str(generic["mensagem_erro"]); msg != "" {
		if code != "" {
			msg = code + ": " + msg
		}
		cons.Motivos = append(cons.Motivos, msg)
	}
	if cons.Status == "" {
		return nil, errors.New("sigcorp: resposta da consulta sem campo status")
	}
	return cons, nil
}

func str(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func firstStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := str(m[k]); s != "" {
			return s
		}
	}
	return ""
}

// flatten turns a string, a list of strings/objects or an object into a list
// of human-readable reason lines.
func flatten(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		if s := strings.TrimSpace(t); s != "" {
			return []string{s}
		}
		return nil
	case []any:
		var out []string
		for _, item := range t {
			out = append(out, flatten(item)...)
		}
		return out
	case map[string]any:
		code := firstStr(t, "codigo", "codigo_erro", "code", "campo", "field")
		msg := firstStr(t, "mensagem", "mensagem_erro", "message", "descricao", "motivo", "detalhe", "erro")
		switch {
		case code != "" && msg != "":
			return []string{code + ": " + msg}
		case msg != "":
			return []string{msg}
		case code != "":
			return []string{code}
		}
		b, err := json.Marshal(t)
		if err != nil {
			return nil
		}
		return []string{string(b)}
	default:
		return []string{fmt.Sprint(t)}
	}
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// HTTPError is a 4xx answer from the platform. Body is kept for diagnostics.
type HTTPError struct {
	URL    string
	Status int
	Body   []byte
}

func (e *HTTPError) Error() string {
	detail := strings.TrimSpace(string(e.Body))
	if m, err := extractMessage(e.Body); err == nil && m != "" {
		detail = m
	}
	if len(detail) > 300 {
		detail = detail[:300] + "…"
	}
	switch e.Status {
	case http.StatusUnauthorized:
		return fmt.Sprintf("sigcorp: HTTP 401 — credenciais recusadas (inscrição municipal, senha ou CNPJ): %s", detail)
	case http.StatusForbidden:
		return fmt.Sprintf("sigcorp: HTTP 403 — inscrição municipal autenticada diverge do prestador da DPS: %s", detail)
	case http.StatusBadRequest:
		return fmt.Sprintf("sigcorp: HTTP 400 — requisição rejeitada (XML, Content-Type ou cabeçalho): %s", detail)
	default:
		return fmt.Sprintf("sigcorp: HTTP %d: %s", e.Status, detail)
	}
}

func extractMessage(body []byte) (string, error) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return "", err
	}
	var parts []string
	if code, msg := str(m["codigo_erro"]), str(m["mensagem_erro"]); msg != "" {
		if code != "" {
			msg = code + ": " + msg
		}
		parts = append(parts, msg)
	}
	for _, key := range []string{"message", "mensagem", "erros", "errors", "motivos", "detalhes"} {
		parts = append(parts, flatten(m[key])...)
	}
	return strings.Join(parts, "; "), nil
}

func httpError(url string, status int, body []byte) error {
	return &HTTPError{URL: url, Status: status, Body: body}
}

// UnavailableError marks transport-level failures (DNS, connection, 5xx):
// the webservice cannot be reached, as opposed to rejecting the request.
type UnavailableError struct {
	URL    string
	Status int
	Err    error
}

func (e *UnavailableError) Error() string {
	reason := ""
	switch {
	case e.Status != 0:
		reason = fmt.Sprintf("HTTP %d", e.Status)
	case e.Err != nil:
		reason = e.Err.Error()
	}
	return fmt.Sprintf("sigcorp: webservice indisponível em %s (%s) — confira a página de integração da prefeitura e a seção [nacional] do config.toml", e.URL, reason)
}

func (e *UnavailableError) Unwrap() error { return e.Err }

func classifyTransportErr(url string, err error) error {
	var dnsErr *net.DNSError
	var opErr *net.OpError
	if errors.As(err, &dnsErr) || errors.As(err, &opErr) {
		return &UnavailableError{URL: url, Err: err}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &UnavailableError{URL: url, Err: err}
	}
	return fmt.Errorf("sigcorp: requisição: %w", err)
}
