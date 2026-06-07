package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/caian-org/nfe/internal/abrasf"
	"github.com/caian-org/nfe/internal/config"
	"github.com/caian-org/nfe/internal/nota"
	"github.com/caian-org/nfe/internal/render"
	"github.com/caian-org/nfe/internal/service"
)

func newEmitCmd(gf *globalFlags) *cobra.Command {
	var (
		dryRun             bool
		verbose            bool
		noConfirmationWait bool
		timeout            time.Duration
	)
	cmd := &cobra.Command{
		Use:   "emit <nota>",
		Short: "Emite uma NFS-e a partir de uma nota do workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			progress := newEmitProgress(cmd, gf, args[0], verbose)
			defer progress.Close()

			progress.Report("nota", service.ProgressStarted, "lendo workspace")
			configPath, err := gf.configPath()
			if err != nil {
				progress.Report("nota", service.ProgressFailed, err.Error())
				return err
			}
			notaPath, err := gf.notaPath(args[0])
			if err != nil {
				progress.Report("nota", service.ProgressFailed, err.Error())
				return err
			}
			cfg, err := config.Load(configPath)
			if err != nil {
				progress.Report("nota", service.ProgressFailed, err.Error())
				return err
			}
			confirmTimeout, confirmInterval, err := cfg.Configuracoes.ConfirmDurations()
			if err != nil {
				progress.Report("nota", service.ProgressFailed, err.Error())
				return err
			}
			input, err := nota.Load(notaPath)
			if err != nil {
				progress.Report("nota", service.ProgressFailed, err.Error())
				return err
			}
			progress.Report("nota", service.ProgressDone, notaPath)

			svc, err := service.New(service.Options{Config: cfg, Progress: progress.ServiceProgress()})
			if err != nil {
				progress.Report("assinatura", service.ProgressFailed, err.Error())
				return err
			}

			ctx, cancel := contextWithTimeout(cmd, timeout)
			defer cancel()

			if dryRun {
				res, err := svc.EmitDryRun(input)
				if err != nil {
					return err
				}
				progress.Report("prefeitura", service.ProgressSkipped, "dry-run")
				progress.Report("confirmação", service.ProgressSkipped, "dry-run")
				progress.Report("config", service.ProgressSkipped, "sem incremento")
				return gf.renderer(cmd).Emit(render.EmitInfo{
					DryRun:    true,
					Verbose:   verbose,
					Sucesso:   true,
					NumeroRPS: res.NumeroRPS,
					SignedXML: string(res.SignedXML),
				})
			}

			res, err := svc.Emit(ctx, input)
			info := render.EmitInfo{Verbose: verbose}
			if res != nil {
				info.NumeroRPS = res.NumeroRPS
				info.NumeroNFSe = res.NumeroNFSe
				info.Mensagens = mapMensagens(res.Mensagens)
				info.Response = string(res.RawResponse)
				info.SignedXML = string(res.SignedXML)
			}
			var msgErr *service.MessagesError
			switch {
			case err == nil:
				info.Sucesso = true
				if info.NumeroNFSe == "" {
					if noConfirmationWait {
						progress.Report("confirmação", service.ProgressSkipped, "não aguardada")
						info.Pendente = true
					} else {
						nfse, confirmXML, ok := waitForEmissionConfirmation(cmd.Context(), svc, res.NumeroRPS, confirmTimeout, confirmInterval, progress)
						info.ConfirmXML = confirmXML
						if ok {
							info.NFSe = nfse
							info.NumeroNFSe = nfse.Numero
						} else {
							info.Pendente = true
						}
					}
				} else {
					progress.Report("confirmação", service.ProgressDone, "confirmada na emissão")
				}
				// Persist the bumped counter only when emit succeeded.
				progress.Report("config", service.ProgressStarted, "salvando contador RPS")
				if saveErr := config.Save(configPath, cfg); saveErr != nil {
					progress.Report("config", service.ProgressFailed, saveErr.Error())
					return fmt.Errorf("emissão concluída mas falhou ao salvar configuração: %w", saveErr)
				}
				progress.Report("config", service.ProgressDone, "contador salvo")
			case errors.As(err, &msgErr):
				info.Sucesso = false
				progress.Report("config", service.ProgressSkipped, "contador preservado")
			default:
				return err
			}
			return gf.renderer(cmd).Emit(info)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "valida o input e gera o XML sem chamar o webservice")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "mostra o XML assinado e a resposta SOAP bruta")
	cmd.Flags().BoolVar(&noConfirmationWait, "no-confirmation-wait", false, "não aguarda a confirmação da NFS-e após envio assíncrono")
	cmd.Flags().DurationVar(&timeout, "wait-timeout", 0, "timeout da requisição (ex.: 30s, 2m); zero usa o padrão")
	return cmd
}

func waitForEmissionConfirmation(ctx context.Context, svc *service.Service, numeroRPS int, timeout, interval time.Duration, progress *emitProgress) (*render.QueriedNFSe, string, bool) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	rps := abrasf.IdentificacaoRps{
		Numero: numeroRPS,
		Serie:  "00000",
		Tipo:   abrasf.TipoRPS,
	}
	var lastResponse string
	attempt := 0
	progress.Report("confirmação", service.ProgressStarted, "consultando RPS")
	for {
		attempt++
		progress.Report("confirmação", service.ProgressStarted, fmt.Sprintf("tentativa %d", attempt))
		res, err := svc.QueryByRPS(ctx, rps)
		if res != nil && len(res.RawResponse) > 0 {
			lastResponse = string(res.RawResponse)
		}
		if err == nil && res != nil {
			nfses := parseQueriedNFSes(res.BodyInner)
			if len(nfses) > 0 {
				progress.Report("confirmação", service.ProgressDone, "NFS-e encontrada")
				return &nfses[0], lastResponse, true
			}
		}

		select {
		case <-ctx.Done():
			progress.Report("confirmação", service.ProgressSkipped, "pendente no prazo configurado")
			return nil, lastResponse, false
		case <-time.After(interval):
		}
	}
}

func contextWithTimeout(cmd *cobra.Command, timeout time.Duration) (ctx context.Context, cancel context.CancelFunc) {
	if timeout > 0 {
		return context.WithTimeout(cmd.Context(), timeout)
	}
	return context.WithCancel(cmd.Context())
}

func mapMensagens(in []abrasf.MensagemRetorno) []render.MensagemRetorno {
	if len(in) == 0 {
		return nil
	}
	out := make([]render.MensagemRetorno, len(in))
	for i, m := range in {
		out[i] = render.MensagemRetorno{Codigo: m.Codigo, Mensagem: m.Mensagem, Correcao: m.Correcao}
	}
	return out
}
