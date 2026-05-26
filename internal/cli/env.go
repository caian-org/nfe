package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/caian-org/nfe/internal/config"
)

func newEnvCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:       "env <homologacao|producao>",
		Short:     "Alterna o ambiente ativo",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{config.EnvHomologacao, config.EnvProducao},
		RunE: func(cmd *cobra.Command, args []string) error {
			env := args[0]
			if env != config.EnvHomologacao && env != config.EnvProducao {
				return fmt.Errorf("ambiente inválido %q: esperado %q ou %q",
					env, config.EnvHomologacao, config.EnvProducao)
			}

			cfg, err := config.Load(gf.configPath)
			if err != nil {
				return err
			}
			cfg.Ambiente = env
			if err := config.Save(gf.configPath, cfg); err != nil {
				return err
			}
			return gf.renderer(cmd).Env(env)
		},
	}
}
