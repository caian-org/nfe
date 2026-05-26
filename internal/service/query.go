package service

import (
	"context"
	"fmt"

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
		return &QueryResult{RequestXML: xmlBytes, RawResponse: resp}, fmt.Errorf("query: %w", err)
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
