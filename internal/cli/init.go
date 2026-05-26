package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"

	"github.com/caian-org/nfe/internal/config"
	"github.com/caian-org/nfe/internal/nota"
)

func newInitCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "init [caminho]",
		Short: "Cria a estrutura inicial de um projeto NFS-e",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := defaultProjectPath()
			if len(args) == 1 {
				path = args[0]
			}
			return runInit(cmd, gf, path)
		},
	}
}

// defaultProjectPath returns "$HOME/.nfews", falling back to "./.nfews" when
// the user home directory cannot be resolved (rare, but possible on minimal
// containers).
func defaultProjectPath() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".nfews")
	}
	return "./.nfews"
}

// defaultConfigPath returns the config.toml inside the default project
// directory. It is used as the fallback for the global -c/--config flag so
// `nfe init` followed by `nfe query` works without any extra arguments.
func defaultConfigPath() string {
	return filepath.Join(defaultProjectPath(), "config.toml")
}

func runInit(cmd *cobra.Command, gf *globalFlags, projectPath string) error {
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		return fmt.Errorf("falha ao criar diretório do projeto: %w", err)
	}

	configPath := filepath.Join(projectPath, "config.toml")
	notaPath := filepath.Join(projectPath, "example-nota.toml")
	readmePath := filepath.Join(projectPath, "README.md")

	if err := config.Save(configPath, config.Default()); err != nil {
		return err
	}

	notaBytes, err := toml.Marshal(nota.Example())
	if err != nil {
		return fmt.Errorf("falha ao serializar nota de exemplo: %w", err)
	}
	if err := os.WriteFile(notaPath, notaBytes, 0o644); err != nil {
		return fmt.Errorf("falha ao escrever nota de exemplo: %w", err)
	}

	if err := os.WriteFile(readmePath, []byte(initReadme), 0o644); err != nil {
		return fmt.Errorf("falha ao escrever README: %w", err)
	}

	return gf.renderer(cmd).Init(projectPath, []string{configPath, notaPath, readmePath})
}

const initReadme = `# Projeto NFS-e ABRASF

Este diretório contém uma configuração e uma nota de exemplo para emitir
NFS-e via webservice ABRASF v2.04.

## Arquivos

- ` + "`config.toml`" + ` — configuração principal (prestador, endpoints, credenciais)
- ` + "`example-nota.toml`" + ` — exemplo de entrada de nota
- ` + "`README.md`" + ` — este arquivo

## Uso

` + "```" + `
nfe emit example-nota.toml          # emite a nota
nfe query --numero 123              # consulta por número
nfe query --data-inicial 2025-01-01 --data-final 2025-01-31
nfe cancel --numero 123 --codigo 1  # cancela a nota
nfe status                          # mostra a configuração ativa
nfe env producao                    # alterna ambiente
` + "```" + `

Edite ` + "`config.toml`" + ` com os dados da sua empresa e credenciais antes de emitir.
`
