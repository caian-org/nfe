package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var notaIDRegex = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// defaultWorkspacePath returns "$HOME/.nfews", falling back to "./.nfews"
// when the user home directory cannot be resolved.
func defaultWorkspacePath() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".nfews")
	}
	return "./.nfews"
}

func (g *globalFlags) workspace() (string, error) {
	abs, err := filepath.Abs(g.workspacePath)
	if err != nil {
		return "", fmt.Errorf("resolver workspace: %w", err)
	}
	return abs, nil
}

func (g *globalFlags) configPath() (string, error) {
	workspace, err := g.workspace()
	if err != nil {
		return "", err
	}
	return filepath.Join(workspace, "config.toml"), nil
}

func (g *globalFlags) notaPath(id string) (string, error) {
	if !notaIDRegex.MatchString(id) {
		return "", fmt.Errorf("identificador da nota inválido %q: use nomes como \"mova\", sem caminho ou extensão .toml", id)
	}
	workspace, err := g.workspace()
	if err != nil {
		return "", err
	}
	return filepath.Join(workspace, "notas", id+".toml"), nil
}
