package cli

import (
	"github.com/spf13/cobra"

	"github.com/caian-org/nfe/internal/config"
	"github.com/caian-org/nfe/internal/render"
)

func newStatusCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Mostra um resumo da configuração ativa",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := gf.configPath()
			if err != nil {
				return err
			}
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			return gf.renderer(cmd).Status(render.NewStatusInfo(cfg))
		},
	}
}
