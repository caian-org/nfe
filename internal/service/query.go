package service

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/caian-org/nfe/internal/abrasf"
	"github.com/caian-org/nfe/internal/soap"
)

// QueryResult is what Query returns.
type QueryResult struct {
	RequestXML  []byte
	RawResponse []byte
	BodyInner   []byte
	Mensagens   []abrasf.MensagemRetorno
}

// Query consults NFS-e by number or by date range against the WS.
func (s *Service) Query(ctx context.Context, q abrasf.ConsultaQuery) (*QueryResult, error) {
	if s.cfg.IsNacional() {
		return nil, fmt.Errorf("query: %w: consulta por número ou período não existe na API da SIGCORP; use --protocolo", ErrNaoSuportadoNacional)
	}
	xmlBytes, err := abrasf.BuildConsultarServicoPrestado(
		s.cfg.Prestador.CNPJ,
		s.cfg.Prestador.InscricaoMunicipal,
		q,
	)
	if err != nil {
		return nil, err
	}

	if err := s.ensureSOAP(); err != nil {
		return nil, err
	}
	resp, err := s.soap.Call(ctx, "ConsultarNfseServicoPrestado", xmlBytes)
	if err != nil {
		result := queryResultFromResponse(xmlBytes, resp)
		if len(result.Mensagens) > 0 {
			return result, &MessagesError{Action: "query", Mensagens: result.Mensagens, Raw: result.BodyInner}
		}
		return result, fmt.Errorf("query: %w", err)
	}

	bodyInner, err := soapExtract(resp)
	if err != nil {
		return nil, fmt.Errorf("query: parse do envelope SOAP: %w", err)
	}
	payload, err := abrasf.ParseResponse(bodyInner)
	if err != nil {
		return nil, fmt.Errorf("query: parse da resposta: %w", err)
	}

	result := &QueryResult{
		RequestXML:  xmlBytes,
		RawResponse: resp,
		BodyInner:   payload.Raw,
		Mensagens:   payload.Mensagens,
	}
	if payload.HasErrors() {
		return result, &MessagesError{Action: "query", Mensagens: payload.Mensagens, Raw: payload.Raw}
	}
	return result, nil
}

// QueryByRPS consults an NFS-e by the RPS identifier used during emission.
func (s *Service) QueryByRPS(ctx context.Context, rps abrasf.IdentificacaoRps) (*QueryResult, error) {
	if s.cfg.IsNacional() {
		return nil, fmt.Errorf("query: %w: consulta por RPS não existe na API da SIGCORP; use --protocolo", ErrNaoSuportadoNacional)
	}
	xmlBytes, err := abrasf.BuildConsultarPorRPS(
		s.cfg.Prestador.CNPJ,
		s.cfg.Prestador.InscricaoMunicipal,
		rps,
	)
	if err != nil {
		return nil, err
	}

	if err := s.ensureSOAP(); err != nil {
		return nil, err
	}
	resp, err := s.soap.Call(ctx, "ConsultarNfsePorRps", xmlBytes)
	if err != nil {
		result := queryResultFromResponse(xmlBytes, resp)
		if len(result.Mensagens) > 0 {
			return result, &MessagesError{Action: "query", Mensagens: result.Mensagens, Raw: result.BodyInner}
		}
		return result, fmt.Errorf("query: %w", err)
	}

	bodyInner, err := soapExtract(resp)
	if err != nil {
		return nil, fmt.Errorf("query: parse do envelope SOAP: %w", err)
	}
	payload, err := abrasf.ParseResponse(bodyInner)
	if err != nil {
		return nil, fmt.Errorf("query: parse da resposta: %w", err)
	}

	result := &QueryResult{
		RequestXML:  xmlBytes,
		RawResponse: resp,
		BodyInner:   payload.Raw,
		Mensagens:   payload.Mensagens,
	}
	if payload.HasErrors() {
		return result, &MessagesError{Action: "query", Mensagens: payload.Mensagens, Raw: payload.Raw}
	}
	return result, nil
}

// soapExtract is a thin shim around soap.ExtractBody, kept in this package
// so the service tests don't have to import it directly.
func soapExtract(envelope []byte) ([]byte, error) {
	return soap.ExtractBody(envelope)
}

func queryResultFromResponse(requestXML, resp []byte) *QueryResult {
	result := &QueryResult{RequestXML: requestXML, RawResponse: resp}
	if len(resp) == 0 {
		return result
	}
	bodyInner, err := soapExtract(resp)
	if err != nil {
		if fault := soapFaultMessage(resp); fault != "" {
			result.Mensagens = []abrasf.MensagemRetorno{{Codigo: "SOAP", Mensagem: fault}}
		}
		return result
	}
	result.BodyInner = bodyInner
	if fault := soapFaultMessage(bodyInner); fault != "" {
		result.Mensagens = []abrasf.MensagemRetorno{{Codigo: "SOAP", Mensagem: fault}}
		return result
	}
	payload, err := abrasf.ParseResponse(bodyInner)
	if err != nil {
		if fault := soapFaultMessage(bodyInner); fault != "" {
			result.Mensagens = []abrasf.MensagemRetorno{{Codigo: "SOAP", Mensagem: fault}}
		}
		return result
	}
	result.BodyInner = payload.Raw
	result.Mensagens = payload.Mensagens
	if len(result.Mensagens) == 0 {
		if fault := soapFaultMessage(payload.Raw); fault != "" {
			result.Mensagens = []abrasf.MensagemRetorno{{Codigo: "SOAP", Mensagem: fault}}
		}
	}
	return result
}

func soapFaultMessage(b []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(b))
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "faultstring" {
			continue
		}
		var msg string
		if err := dec.DecodeElement(&msg, &se); err != nil {
			return ""
		}
		return strings.TrimSpace(msg)
	}
}
