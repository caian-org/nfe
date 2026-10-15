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

const initReadme = `# Projeto NFS-e

Este diretório contém uma configuração e uma nota de exemplo para emitir
NFS-e no layout nacional pelo WS Padrão Nacional da SIGCORP (Franco da
Rocha). Removendo a seção ` + "`[nacional]`" + ` do ` + "`config.toml`" + ` e preenchendo
` + "`[soap]`" + `, o mesmo workspace fala com webservices ABRASF v2.04.

## Arquivos

- ` + "`config.toml`" + ` — configuração principal (prestador, credenciais, códigos tributários)
- ` + "`notas/example.toml`" + ` — exemplo de entrada de nota
- ` + "`README.md`" + ` — este arquivo

## Uso

` + "```" + `
nfe emit example --dry-run          # monta e assina a DPS sem enviar
nfe emit example                    # envia notas/example.toml e aguarda a aprovação
nfe emit example --no-confirmation-wait
nfe query --protocolo <protocolo>   # reconsulta uma emissão
nfe status                          # mostra a configuração ativa
nfe env producao                    # alterna ambiente
` + "```" + `

Edite ` + "`config.toml`" + ` com os dados da sua empresa, a senha do portal da prefeitura
e os códigos de ` + "`[ibscbs]`" + ` antes de emitir. Em cada nota informe ` + "`cnbs`" + ` e
` + "`servico.codigo_tributacao_nacional`" + ` (planilha de serviços da prefeitura).
O ` + "`emit`" + ` salva ` + "`proximo_numero_rps`" + ` e ` + "`proximo_numero_nfse`" + ` assim que a
prefeitura aceita a DPS e aguarda a aprovação usando ` + "`confirm_timeout`" + ` e
` + "`confirm_interval`" + ` em ` + "`[configuracoes]`" + `.
Durante a emissão, o progresso aparece no terminal; com ` + "`--json`" + `, a saída permanece apenas JSON.
`
