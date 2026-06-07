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
			path := gf.workspacePath
			if len(args) == 1 {
				path = args[0]
			}
			return runInit(cmd, gf, path)
		},
	}
}

func runInit(cmd *cobra.Command, gf *globalFlags, projectPath string) error {
	projectPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("resolver workspace: %w", err)
	}
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		return fmt.Errorf("falha ao criar diretório do projeto: %w", err)
	}

	configPath := filepath.Join(projectPath, "config.toml")
	notasPath := filepath.Join(projectPath, "notas")
	notaPath := filepath.Join(notasPath, "example.toml")
	readmePath := filepath.Join(projectPath, "README.md")

	if err := os.MkdirAll(notasPath, 0o755); err != nil {
		return fmt.Errorf("falha ao criar diretório de notas: %w", err)
	}

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
- ` + "`notas/example.toml`" + ` — exemplo de entrada de nota
- ` + "`README.md`" + ` — este arquivo

## Uso

` + "```" + `
nfe emit example                    # emite notas/example.toml
nfe emit example --no-confirmation-wait
nfe query --numero 123              # consulta por número
nfe query --data-inicial 2025-01-01 --data-final 2025-01-31
nfe cancel --numero 123 --codigo 1  # cancela a nota
nfe status                          # mostra a configuração ativa
nfe env producao                    # alterna ambiente
` + "```" + `

Edite ` + "`config.toml`" + ` com os dados da sua empresa e credenciais antes de emitir.
O ` + "`emit`" + ` aguarda confirmação por RPS usando ` + "`confirm_timeout`" + ` e ` + "`confirm_interval`" + ` em ` + "`[configuracoes]`" + `.
Durante a emissão, o progresso aparece no terminal; com ` + "`--json`" + `, a saída permanece apenas JSON.
`
