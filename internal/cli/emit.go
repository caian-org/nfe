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
		dryRun  bool
		timeout time.Duration
	)
	cmd := &cobra.Command{
		Use:   "emit <arquivo>",
		Short: "Emite uma NFS-e a partir de um arquivo TOML",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(gf.configPath)
			if err != nil {
				return err
			}
			input, err := nota.Load(args[0])
			if err != nil {
				return err
			}

			svc, err := service.New(service.Options{Config: cfg})
			if err != nil {
				return err
			}

			ctx, cancel := contextWithTimeout(cmd, timeout)
			defer cancel()

			if dryRun {
				res, err := svc.EmitDryRun(input)
				if err != nil {
					return err
				}
				return gf.renderer(cmd).Emit(render.EmitInfo{
					DryRun:    true,
					Sucesso:   true,
					NumeroRPS: res.NumeroRPS,
					SignedXML: string(res.SignedXML),
				})
			}

			res, err := svc.Emit(ctx, input)
			info := render.EmitInfo{}
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
				// Persist the bumped counter only when emit succeeded.
				if saveErr := config.Save(gf.configPath, cfg); saveErr != nil {
					return fmt.Errorf("emissão concluída mas falhou ao salvar configuração: %w", saveErr)
				}
			case errors.As(err, &msgErr):
				info.Sucesso = false
			default:
				return err
			}
			return gf.renderer(cmd).Emit(info)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "valida o input e gera o XML sem chamar o webservice")
	cmd.Flags().DurationVar(&timeout, "wait-timeout", 0, "timeout da requisição (ex.: 30s, 2m); zero usa o padrão")
	return cmd
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
