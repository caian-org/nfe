package cli

import (
	"github.com/spf13/cobra"

	"github.com/caian-org/nfe/internal/render"
)

// Version, Commit and BuildDate are overridden at release time via
// goreleaser's ldflags (`-X github.com/caian-org/nfe/internal/cli.Version=<tag>`).
var (
	Version   = "0.1.0-dev"
	Commit    = ""
	BuildDate = ""
)

type globalFlags struct {
	workspacePath string
	json          bool
}

func (g *globalFlags) renderer(cmd *cobra.Command) render.Renderer {
	return render.New(g.json, cmd.OutOrStdout())
}

// NewRoot returns the root cobra command with all subcommands wired in.
func NewRoot() *cobra.Command {
	gf := &globalFlags{}

	root := &cobra.Command{
		Use:           "nfe",
		Short:         "CLI para NFS-e ABRASF",
		Long:          "Cliente de linha de comando para emissão, consulta e cancelamento de NFS-e seguindo o padrão ABRASF v2.04.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: false,
	}

	root.PersistentFlags().StringVarP(&gf.workspacePath, "workspace", "w", defaultWorkspacePath(), "diretório de trabalho com config.toml e notas/")
	root.PersistentFlags().BoolVar(&gf.json, "json", false, "emite saída JSON ao invés de texto humano")

	root.AddCommand(
		newInitCmd(gf),
		newEnvCmd(gf),
		newStatusCmd(gf),
		newEmitCmd(gf),
		newQueryCmd(gf),
		newCancelCmd(gf),
	)

	return root
}
