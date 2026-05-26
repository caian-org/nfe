---
name: nf-cli-translation
description: pt-BR voice and error-message conventions for nf. Use when adding or editing Cobra command descriptions, error wrappers, render labels, or any other user-visible string.
---

# Nf CLI Translation

The CLI and all user-facing output are in pt-BR. Agent-facing prose
(AGENTS.md, CLAUDE.md, SKILL.md, agent prompts), code comments, and
package doc strings stay in English. This skill documents how the split
is enforced in practice.

## Error wrapping

Pattern: package prefix in English, body in pt-BR.

```go
fmt.Errorf("xmlsig: documento vazio")
fmt.Errorf("soap: falha ao montar envelope")
fmt.Errorf("wsdl: targetNamespace não encontrado")
fmt.Errorf("emit: assinatura: %w", err)
```

Why: the prefix is diagnostic (the user can grep code by it), the body
is informational (the user reads it on the terminal). Errors percolate
through `fmt.Errorf("%w", ...)` wrapping; each layer adds its own
prefix and pt-BR clause.

Standard prefixes in use:

| Prefix      | Package                  |
|-------------|--------------------------|
| `xmlsig:`   | `internal/xmlsig`        |
| `soap:`     | `internal/soap`          |
| `wsdl:`     | `internal/soap` (WSDL)   |
| `emit:`     | `internal/service/emit`  |
| `query:`    | `internal/service/query` |
| `cancel:`   | `internal/service/cancel`|
| `nota:`     | `internal/nota`          |
| `config:`   | `internal/config`        |

When introducing a new error in an existing package, reuse the existing
prefix. When introducing a new package, pick a short English prefix
that does not collide.

## Cobra help text

`Short`, `Long`, and flag descriptions are pt-BR.

```go
root := &cobra.Command{
    Use:   "nfe",
    Short: "CLI para NFS-e ABRASF",
    Long:  "Cliente de linha de comando para emissão, consulta e cancelamento de NFS-e seguindo o padrão ABRASF v2.04.",
}
root.PersistentFlags().StringVarP(&gf.configPath, "config", "c",
    defaultConfigPath(), "caminho do arquivo de configuração")
```

Style: imperative verbs for `Short` ("Emite", "Consulta", "Cancela"),
descriptive sentences for `Long`. No trailing period on `Short`; full
sentence with terminating period on `Long`.

## Render labels

`internal/render/human.go` standardises the labels that prefix output
lines:

- `OK:` — success path. Kept English; "OK" is universally readable and
  the existing convention.
- `ERRO:` — error path.
- `dica:` — suggestion / next-step pointer.

Status messages are short pt-BR sentences:

- `projeto inicializado em <path>`
- `ambiente alterado para <name>`
- `falha na emissão`
- `falha na consulta`
- `falha no cancelamento`
- `nenhuma NFS-e encontrada`
- `dry-run — sem chamada SOAP, sem incremento do contador`

When adding a new label, follow the same lowercase-pt-BR style.

## README and embedded init README

The user-facing README at the repo root is pt-BR. The `nfe init`
command writes a project-local README (`initReadme` constant in
`internal/cli/init.go`) — that one is also pt-BR.

## When in doubt

- Is the string ever shown to a Brazilian end user (terminal output,
  README, init scaffold)? → pt-BR.
- Is the string read only by agents/maintainers (CLAUDE.md, AGENTS.md,
  code comments, SKILL.md, agent prompts)? → English.
- Is the string an internal identifier (struct tag, prefix, enum
  constant)? → English.

If the same noun appears in both audiences (e.g. "emissão" vs
"emission"), prefer the audience's natural form on each side.
