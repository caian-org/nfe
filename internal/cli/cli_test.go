package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/caian-org/nfe/internal/cli"
	"github.com/caian-org/nfe/internal/config"
)

// runCmd executes the root cobra command with args, returning captured stdout
// and any error from Execute.
func runCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := cli.NewRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestInitCreatesProject(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proj")
	out, err := runCmd(t, "init", dir)
	require.NoError(t, err)
	assert.Contains(t, out, "projeto inicializado em")

	cfgPath := filepath.Join(dir, "config.toml")
	notaPath := filepath.Join(dir, "example-nota.toml")
	readmePath := filepath.Join(dir, "README.md")
	for _, p := range []string{cfgPath, notaPath, readmePath} {
		assert.FileExists(t, p)
	}

	// The scaffolded config must be a valid Config.
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, config.EnvHomologacao, cfg.Ambiente)
}

func TestInitDefaultPathUsesNfewsUnderHome(t *testing.T) {
	// Point HOME at a temp dir so the default doesn't try to write into the
	// caller's real home directory.
	t.Setenv("HOME", t.TempDir())
	out, err := runCmd(t, "init")
	require.NoError(t, err)

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	expected := filepath.Join(home, ".nfews")
	assert.Contains(t, out, expected)
	assert.FileExists(t, filepath.Join(expected, "config.toml"))
}

// After `nfe init` without args, subsequent `nfe status` (no -c) must pick up
// the config that init just wrote into ~/.nfews — otherwise the default flow
// is broken from the user's perspective.
func TestStatusDefaultConfigResolvesNfewsUnderHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := runCmd(t, "init")
	require.NoError(t, err)

	out, err := runCmd(t, "status")
	require.NoError(t, err)
	assert.Contains(t, out, "homologacao")
}

func TestInitJSON(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proj")
	out, err := runCmd(t, "--json", "init", dir)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, "init", got["event"])
	assert.Equal(t, dir, got["path"])

	created, ok := got["created"].([]any)
	require.True(t, ok)
	assert.Len(t, created, 3)
}

func TestEnvSwitch(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proj")
	_, err := runCmd(t, "init", dir)
	require.NoError(t, err)

	cfgPath := filepath.Join(dir, "config.toml")
	out, err := runCmd(t, "-c", cfgPath, "env", "producao")
	require.NoError(t, err)
	assert.Contains(t, out, "ambiente alterado para producao")

	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, config.EnvProducao, cfg.Ambiente)
}

func TestEnvRejectsInvalid(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proj")
	_, err := runCmd(t, "init", dir)
	require.NoError(t, err)

	cfgPath := filepath.Join(dir, "config.toml")
	_, err = runCmd(t, "-c", cfgPath, "env", "qa")
	require.Error(t, err)
}

func TestStatusHuman(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proj")
	_, err := runCmd(t, "init", dir)
	require.NoError(t, err)

	cfgPath := filepath.Join(dir, "config.toml")
	out, err := runCmd(t, "-c", cfgPath, "status")
	require.NoError(t, err)
	assert.Contains(t, out, "ambiente")
	assert.Contains(t, out, "homologacao")
	assert.Contains(t, out, "5.00%")
	// No emojis allowed anywhere in human output.
	for _, r := range []rune{'✅', '❌', '🔄', '📤', '⏳', '📄'} {
		assert.NotContains(t, out, string(r), "human output must not contain emoji %q", string(r))
	}
}

func TestStatusJSON(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proj")
	_, err := runCmd(t, "init", dir)
	require.NoError(t, err)

	cfgPath := filepath.Join(dir, "config.toml")
	out, err := runCmd(t, "--json", "-c", cfgPath, "status")
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, "status", got["event"])

	status, ok := got["status"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "homologacao", status["ambiente"])
	assert.True(t, status["using_basic_auth"].(bool))
	assert.False(t, status["using_certificate"].(bool))
}

func TestStatusMissingConfigErrors(t *testing.T) {
	_, err := runCmd(t, "-c", "/no/such/dir/config.toml", "status")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "não encontrado")
}
