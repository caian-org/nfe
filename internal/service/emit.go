package service

import (
	"context"
	"fmt"

	"github.com/caian-org/nfe/internal/abrasf"
	"github.com/caian-org/nfe/internal/nota"
)

// EmitResult is what Emit returns on success. On failure (validation, network,
// or business rules) Emit returns an error and *EmitResult is nil.
type EmitResult struct {
	NumeroRPS    int    // Sequence number used for this RPS (already bumped on success).
	NumeroNFSe   string // Authorized NFS-e number returned by the WS, when present.
	UnsignedXML  []byte // The XML pre-signing — useful for debugging.
	SignedXML    []byte // The XML actually posted (or that would be posted on dry-run).
	RawResponse  []byte // The SOAP response body returned by the WS. Empty on dry-run.
	Mensagens    []abrasf.MensagemRetorno
}

// Emit posts a new NFS-e to the WS. On success it bumps cfg.Configuracoes.
// ProximoNumeroRPS but does NOT persist the config back to disk; the caller
// (CLI) is responsible for invoking config.Save once Emit returns.
func (s *Service) Emit(ctx context.Context, in *nota.Input) (*EmitResult, error) {
	if errs := nota.Validate(in); len(errs) > 0 {
		return nil, fmt.Errorf("input inválido: %s", joinErrors(errs))
	}

	inf := abrasf.BuildRPS(s.cfg, in, s.now())
	unsigned, err := abrasf.BuildGerarNfse(inf)
	if err != nil {
		return nil, err
	}
	signed := unsigned
	if s.signer != nil {
		signed, err = s.signer.Sign(unsigned, "InfDeclaracaoPrestacaoServico", inf.ID)
		if err != nil {
			return nil, fmt.Errorf("emit: assinatura: %w", err)
		}
	}

	if err := s.ensureSOAP(); err != nil {
		return nil, err
	}
	resp, err := s.soap.Call(ctx, "GerarNfse", signed)
	if err != nil {
		return &EmitResult{
			NumeroRPS:   inf.Rps.IdentificacaoRps.Numero,
			UnsignedXML: unsigned,
			SignedXML:   signed,
			RawResponse: resp,
		}, fmt.Errorf("emit: %w", err)
	}

	bodyInner, err := soapExtract(resp)
	if err != nil {
		return nil, fmt.Errorf("emit: parse do envelope SOAP: %w", err)
	}
	payload, err := abrasf.ParseResponse(bodyInner)
	if err != nil {
		return nil, fmt.Errorf("emit: parse da resposta: %w", err)
	}

	result := &EmitResult{
		NumeroRPS:   inf.Rps.IdentificacaoRps.Numero,
		UnsignedXML: unsigned,
		SignedXML:   signed,
		RawResponse: resp,
		Mensagens:   payload.Mensagens,
	}

	if payload.HasErrors() {
		return result, &MessagesError{Action: "emit", Mensagens: payload.Mensagens, Raw: payload.Raw}
	}

	result.NumeroNFSe = abrasf.FindNFSeNumero(payload.Raw)
	s.cfg.Configuracoes.ProximoNumeroRPS++
	return result, nil
}

// EmitDryRun produces the signed XML that Emit would post, without touching
// the network or bumping the RPS counter. Used by `nfe emit --dry-run`.
func (s *Service) EmitDryRun(in *nota.Input) (*EmitResult, error) {
	if errs := nota.Validate(in); len(errs) > 0 {
		return nil, fmt.Errorf("input inválido: %s", joinErrors(errs))
	}
	inf := abrasf.BuildRPS(s.cfg, in, s.now())
	unsigned, err := abrasf.BuildGerarNfse(inf)
	if err != nil {
		return nil, err
	}
	signed := unsigned
	if s.signer != nil {
		signed, err = s.signer.Sign(unsigned, "InfDeclaracaoPrestacaoServico", inf.ID)
		if err != nil {
			return nil, fmt.Errorf("emit: assinatura: %w", err)
		}
	}
	return &EmitResult{
		NumeroRPS:   inf.Rps.IdentificacaoRps.Numero,
		UnsignedXML: unsigned,
		SignedXML:   signed,
	}, nil
}

func joinErrors(errs []string) string {
	out := ""
	for i, e := range errs {
		if i > 0 {
			out += "; "
		}
		out += e
	}
	return out
}
