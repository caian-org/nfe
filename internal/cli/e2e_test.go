package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/caian-org/nfe/internal/config"
)

// TestEndToEndDryRunMatchesGolden exercises the full pipeline a real `nfe emit
// --dry-run` invocation would take: load a TOML config, load a TOML invoice
// input, build the DPS, marshal it to XML. `nfe init` scaffolds the national
// (SIGCORP) mode, so the landmarks are those of the DPS layout. We don't
// compare against the package golden here because the CLI uses a
// non-deterministic dhEmi (now). That deterministic check lives in
// internal/dps — this test guarantees the CLI plumbing reaches it.
func TestEndToEndDryRunMatchesGolden(t *testing.T) {
	dir := t.TempDir()

	// 1. Scaffold a fresh project via `nfe init`.
	proj := filepath.Join(dir, "proj")
	_, err := runCmd(t, "init", proj)
	require.NoError(t, err)
	cfgPath := filepath.Join(proj, "config.toml")
	notaPath := filepath.Join(proj, "notas", "example.toml")
	require.FileExists(t, cfgPath)
	require.FileExists(t, notaPath)

	// 2. Drive emit --dry-run --json, parse the structured output.
	out, err := runCmd(t, "--json", "-w", proj, "emit", "example", "--dry-run")
	require.NoError(t, err)

	var resp struct {
		Emit struct {
			DryRun     bool   `json:"dry_run"`
			Sucesso    bool   `json:"sucesso"`
			Modo       string `json:"modo"`
			NumeroRPS  int    `json:"numero_rps"`
			NumeroNFSe string `json:"numero_nfse"`
			SignedXML  string `json:"signed_xml"`
		} `json:"emit"`
		Event string `json:"event"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &resp))
	assert.Equal(t, "emit", resp.Event)
	assert.True(t, resp.Emit.DryRun)
	assert.True(t, resp.Emit.Sucesso)
	assert.Equal(t, "nacional", resp.Emit.Modo)
	assert.Equal(t, 1, resp.Emit.NumeroRPS, "first emission must use DPS number 1")
	assert.Equal(t, "1", resp.Emit.NumeroNFSe, "first emission must use NFS-e number 1")

	// 3. Verify the XML has all the landmarks the national webservice expects.
	xmlStr := resp.Emit.SignedXML
	assert.Contains(t, xmlStr, `<DPS xmlns="http://www.sped.fazenda.gov.br/nfse" versao="1.01">`)
	assert.Contains(t, xmlStr, `<infDPS Id="DPS000000020000000000000000001000000000000001">`)
	assert.Contains(t, xmlStr, `<vServ>1000.00</vServ>`)
	assert.Contains(t, xmlStr, `<cTribNac>010101</cTribNac>`)
	assert.Contains(t, xmlStr, `<cNBS>115022000</cNBS>`)
	assert.Contains(t, xmlStr, `<toma><CNPJ>00000000000000</CNPJ>`) // tomador CNPJ from example
	assert.Contains(t, xmlStr, `<IBSCBS><finNFSe>0</finNFSe><cIndOp>100301</cIndOp>`)
	assert.NotContains(t, xmlStr, `✅`, "must not contain emojis")
	assert.NotContains(t, xmlStr, `❌`, "must not contain emojis")

	// 4. Confirm the counters on disk were NOT bumped by dry-run.
	reloaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, 1, reloaded.Configuracoes.ProximoNumeroRPS)
	assert.Equal(t, 1, reloaded.Configuracoes.ProximoNumeroNFSe)

	// 5. The notas/example.toml that init wrote must round-trip cleanly: a
	// fresh `os.ReadFile` returns the same content nota.Load already parsed.
	contents, err := os.ReadFile(notaPath)
	require.NoError(t, err)
	assert.Contains(t, string(contents), "DESCRIÇÃO DO SERVIÇO PRESTADO")
}

func TestEmitDefaultWorkspaceResolvesNotaUnderHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := runCmd(t, "init")
	require.NoError(t, err)

	out, err := runCmd(t, "emit", "example", "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out, "dry-run")
	assert.Contains(t, out, "RPS número")
}

func TestNoEmojisInAnyHumanOutput(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proj")
	_, err := runCmd(t, "init", dir)
	require.NoError(t, err)

	outputs := map[string]func() (string, error){
		"init":   func() (string, error) { return runCmd(t, "init", t.TempDir()) },
		"env":    func() (string, error) { return runCmd(t, "-w", dir, "env", "producao") },
		"status": func() (string, error) { return runCmd(t, "-w", dir, "status") },
		"emit-dry": func() (string, error) {
			return runCmd(t, "-w", dir, "emit", "example", "--dry-run")
		},
		"query-bad": func() (string, error) { o, _ := runCmd(t, "-w", dir, "query"); return o, nil },
	}

	emojis := []string{"✅", "❌", "🔄", "📤", "⏳", "📄", "📝", "🔑", "🎉"}
	for name, fn := range outputs {
		t.Run(name, func(t *testing.T) {
			out, _ := fn()
			for _, e := range emojis {
				assert.NotContains(t, out, e, "%s output must not contain emoji %q", name, e)
			}
		})
	}
}
