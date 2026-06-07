package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/caian-org/nfe/internal/abrasf"
	"github.com/caian-org/nfe/internal/config"
	"github.com/caian-org/nfe/internal/render"
	"github.com/caian-org/nfe/internal/service"
)

func newCancelCmd(gf *globalFlags) *cobra.Command {
	var (
		numero  string
		codigo  int
		timeout time.Duration
	)
	cmd := &cobra.Command{
		Use:   "cancel",
		Short: "Cancela uma NFS-e autorizada anteriormente",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if numero == "" {
				return errors.New("--numero é obrigatório")
			}
			if codigo < abrasf.CancelErroEmissao || codigo > abrasf.CancelDuplicidade {
				return fmt.Errorf("--codigo deve estar entre %d e %d",
					abrasf.CancelErroEmissao, abrasf.CancelDuplicidade)
			}

			configPath, err := gf.configPath()
			if err != nil {
				return err
			}
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			svc, err := service.New(service.Options{Config: cfg})
			if err != nil {
				return err
			}
			ctx, cancel := contextWithTimeout(cmd, timeout)
			defer cancel()

			res, err := svc.Cancel(ctx, numero, codigo)
			info := render.CancelInfo{NumeroNFSe: numero, Codigo: codigo}
			if res != nil {
				info.Mensagens = mapMensagens(res.Mensagens)
				info.Response = string(res.BodyInner)
			}
			var msgErr *service.MessagesError
			switch {
			case err == nil:
				info.Sucesso = true
			case errors.As(err, &msgErr):
				info.Sucesso = false
			default:
				return err
			}
			return gf.renderer(cmd).Cancel(info)
		},
	}
	cmd.Flags().StringVarP(&numero, "numero", "n", "", "número da NFS-e a cancelar (obrigatório)")
	cmd.Flags().IntVar(&codigo, "codigo", 0, "código de cancelamento: 1=erro emissão, 2=serviço não prestado, 3=erro processamento, 4=duplicidade")
	_ = cmd.MarkFlagRequired("numero")
	_ = cmd.MarkFlagRequired("codigo")
	return cmd
}
