package service

import (
	"context"
	"fmt"

	"github.com/caian-org/nfe/internal/abrasf"
)

// CancelResult is what Cancel returns.
type CancelResult struct {
	NumeroNFSe  string
	UnsignedXML []byte
	SignedXML   []byte
	RawResponse []byte
	BodyInner   []byte
	Mensagens   []abrasf.MensagemRetorno
}

// Cancel posts a CancelarNfseEnvio referencing the given NFS-e number.
func (s *Service) Cancel(ctx context.Context, numeroNFSe string, codigo int) (*CancelResult, error) {
	in := abrasf.CancelInput{
		NumeroNfse:         numeroNFSe,
		CNPJ:               s.cfg.Prestador.CNPJ,
		InscricaoMunicipal: s.cfg.Prestador.InscricaoMunicipal,
		CodigoMunicipio:    s.cfg.Configuracoes.CodigoMunicipio,
		Codigo:             codigo,
	}
	unsigned, err := abrasf.BuildCancelarNfse(in)
	if err != nil {
		return nil, err
	}
	signed := unsigned
	if s.signer != nil {
		signed, err = s.signer.Sign(unsigned, "InfPedidoCancelamento", "")
		if err != nil {
			return nil, fmt.Errorf("cancel: assinatura: %w", err)
		}
	}

	if err := s.ensureSOAP(); err != nil {
		return nil, err
	}
	resp, err := s.soap.Call(ctx, "CancelarNfse", signed)
	if err != nil {
		return &CancelResult{
			NumeroNFSe:  numeroNFSe,
			UnsignedXML: unsigned,
			SignedXML:   signed,
			RawResponse: resp,
		}, fmt.Errorf("cancel: %w", err)
	}

	bodyInner, err := soapExtract(resp)
	if err != nil {
		return nil, fmt.Errorf("cancel: parse do envelope SOAP: %w", err)
	}
	payload, err := abrasf.ParseResponse(bodyInner)
	if err != nil {
		return nil, fmt.Errorf("cancel: parse da resposta: %w", err)
	}

	result := &CancelResult{
		NumeroNFSe:  numeroNFSe,
		UnsignedXML: unsigned,
		SignedXML:   signed,
		RawResponse: resp,
		BodyInner:   payload.Raw,
		Mensagens:   payload.Mensagens,
	}
	if payload.HasErrors() {
		return result, &MessagesError{Action: "cancel", Mensagens: payload.Mensagens, Raw: payload.Raw}
	}
	return result, nil
}
