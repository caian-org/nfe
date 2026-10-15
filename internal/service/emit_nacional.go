package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/caian-org/nfe/internal/abrasf"
	"github.com/caian-org/nfe/internal/dps"
	"github.com/caian-org/nfe/internal/nota"
	"github.com/caian-org/nfe/internal/sigcorp"
)

// ConfirmResult is the outcome of polling a SIGCORP protocol.
type ConfirmResult struct {
	Protocolo     string
	Status        string
	Final         bool
	NFSe          *dps.NFSe
	NFSeXML       string
	LinkImpressao string
	Mensagens     []abrasf.MensagemRetorno
	RawResponse   []byte // last consulta body
}

func (s *Service) buildDPS(in *nota.Input) (*dps.DPS, []byte, []byte, error) {
	s.report("xml", ProgressStarted, "montando DPS")
	d, err := dps.Build(s.cfg, in, s.now())
	if err != nil {
		s.report("xml", ProgressFailed, err.Error())
		return nil, nil, nil, err
	}
	unsigned, err := dps.Marshal(d)
	if err != nil {
		s.report("xml", ProgressFailed, err.Error())
		return nil, nil, nil, err
	}
	s.report("xml", ProgressDone, fmt.Sprintf("DPS %s série %s", d.InfDPS.NDPS, d.InfDPS.Serie))

	signed := unsigned
	switch {
	case s.signer == nil:
		s.report("assinatura", ProgressSkipped, "sem certificado configurado")
	case !s.cfg.AssinarDPS():
		s.report("assinatura", ProgressSkipped, "desligada em nacional.assinar_dps")
	default:
		s.report("assinatura", ProgressStarted, "assinando infDPS")
		signed, err = s.signer.SignSibling(unsigned, "infDPS", d.InfDPS.ID)
		if err != nil {
			s.report("assinatura", ProgressFailed, err.Error())
			return nil, nil, nil, fmt.Errorf("emit: assinatura: %w", err)
		}
		s.report("assinatura", ProgressDone, "DPS assinada")
	}
	return d, unsigned, signed, nil
}

func (s *Service) emitDryRunNacional(in *nota.Input) (*EmitResult, error) {
	_, unsigned, signed, err := s.buildDPS(in)
	if err != nil {
		return nil, err
	}
	return &EmitResult{
		NumeroRPS:   s.cfg.Configuracoes.ProximoNumeroRPS,
		NumeroNFSe:  strconv.Itoa(s.cfg.Configuracoes.ProximoNumeroNFSe),
		UnsignedXML: unsigned,
		SignedXML:   signed,
	}, nil
}

// emitNacional posts the DPS to the SIGCORP recepção endpoint. A 202 consumes
// both counters (the platform rejects a reused nNFSe), so they are bumped here
// even though the NFS-e is only confirmed later by WaitConfirmacao.
func (s *Service) emitNacional(ctx context.Context, in *nota.Input) (*EmitResult, error) {
	_, unsigned, signed, err := s.buildDPS(in)
	if err != nil {
		return nil, err
	}
	nNFSe := strconv.Itoa(s.cfg.Configuracoes.ProximoNumeroNFSe)
	result := &EmitResult{
		NumeroRPS:   s.cfg.Configuracoes.ProximoNumeroRPS,
		NumeroNFSe:  nNFSe,
		UnsignedXML: unsigned,
		SignedXML:   signed,
	}

	s.report("prefeitura", ProgressStarted, "preparando envio")
	if err := s.ensureREST(); err != nil {
		s.report("prefeitura", ProgressFailed, err.Error())
		return nil, err
	}
	s.report("prefeitura", ProgressStarted, "enviando DPS (NFS-e "+nNFSe+")")
	rec, raw, err := s.rest.Recepcionar(ctx, nNFSe, signed)
	result.RawResponse = raw
	if err != nil {
		s.report("prefeitura", ProgressFailed, err.Error())
		var he *sigcorp.HTTPError
		if errors.As(err, &he) {
			result.Mensagens = []abrasf.MensagemRetorno{{Codigo: "HTTP " + strconv.Itoa(he.Status), Mensagem: err.Error()}}
			return result, &MessagesError{Action: "emit", Mensagens: result.Mensagens, Raw: raw}
		}
		return result, fmt.Errorf("emit: %w", err)
	}
	result.Protocolo = rec.Protocolo
	s.report("prefeitura", ProgressDone, "protocolo "+rec.Protocolo)

	s.cfg.Configuracoes.ProximoNumeroRPS++
	s.cfg.Configuracoes.ProximoNumeroNFSe++
	return result, nil
}

// WaitConfirmacao polls the consulta endpoint until the protocol reaches a
// final status or timeout elapses. A rejection is returned as *MessagesError
// alongside the result so callers can render the reasons.
func (s *Service) WaitConfirmacao(ctx context.Context, protocolo string, timeout, interval time.Duration) (*ConfirmResult, error) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	if err := s.ensureREST(); err != nil {
		return nil, err
	}

	res := &ConfirmResult{Protocolo: protocolo}
	attempt := 0
	for {
		attempt++
		s.report("confirmação", ProgressStarted, fmt.Sprintf("consultando protocolo (tentativa %d)", attempt))
		cons, raw, err := s.rest.Consultar(ctx, protocolo)
		if len(raw) > 0 {
			res.RawResponse = raw
		}
		if err == nil {
			s.applyConsulta(res, cons)
			if cons.Final() {
				return s.finishConfirmacao(res)
			}
		} else {
			var ue *sigcorp.UnavailableError
			if !errors.As(err, &ue) {
				s.report("confirmação", ProgressFailed, err.Error())
				return res, fmt.Errorf("confirmação: %w", err)
			}
		}

		select {
		case <-ctx.Done():
			s.report("confirmação", ProgressSkipped, "pendente no prazo configurado")
			return res, nil
		case <-time.After(interval):
		}
	}
}

// QueryProtocolo performs a single consulta for `nfe query --protocolo`.
func (s *Service) QueryProtocolo(ctx context.Context, protocolo string) (*ConfirmResult, error) {
	if err := s.ensureREST(); err != nil {
		return nil, err
	}
	cons, raw, err := s.rest.Consultar(ctx, protocolo)
	res := &ConfirmResult{Protocolo: protocolo, RawResponse: raw}
	if err != nil {
		var he *sigcorp.HTTPError
		if errors.As(err, &he) {
			res.Mensagens = []abrasf.MensagemRetorno{{Codigo: "HTTP " + strconv.Itoa(he.Status), Mensagem: err.Error()}}
			return res, &MessagesError{Action: "query", Mensagens: res.Mensagens, Raw: raw}
		}
		return res, fmt.Errorf("query: %w", err)
	}
	s.applyConsulta(res, cons)
	if !cons.Final() {
		return res, nil
	}
	return s.finishConfirmacao(res)
}

func (s *Service) applyConsulta(res *ConfirmResult, cons *sigcorp.Consulta) {
	res.Status = cons.Status
	res.Final = cons.Final()
	res.NFSeXML = cons.NFSeXML
	res.LinkImpressao = cons.LinkImpressao
	res.Mensagens = nil
	for _, m := range cons.Motivos {
		res.Mensagens = append(res.Mensagens, abrasf.MensagemRetorno{Codigo: cons.Status, Mensagem: m})
	}
	if cons.Status == sigcorp.StatusAprovado && cons.NFSeXML != "" {
		if n, err := dps.ParseNFSe([]byte(cons.NFSeXML)); err == nil {
			res.NFSe = n
		}
	}
}

func (s *Service) finishConfirmacao(res *ConfirmResult) (*ConfirmResult, error) {
	if res.Status == sigcorp.StatusAprovado {
		detail := "NFS-e aprovada"
		if res.NFSe != nil && res.NFSe.Numero != "" {
			detail = "NFS-e " + res.NFSe.Numero + " aprovada"
		}
		s.report("confirmação", ProgressDone, detail)
		return res, nil
	}
	if len(res.Mensagens) == 0 {
		res.Mensagens = []abrasf.MensagemRetorno{{Codigo: res.Status, Mensagem: "sem detalhes na resposta da consulta"}}
	}
	s.report("confirmação", ProgressFailed, res.Status)
	return res, &MessagesError{Action: "emit", Mensagens: res.Mensagens, Raw: res.RawResponse}
}
