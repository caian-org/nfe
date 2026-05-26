---
name: nf-dev-workflow
description: Local development workflow for the nf repository. Use when orienting in the codebase, choosing the right just recipe, picking a validation lane, or making cross-package changes that are not tied to a single domain skill.
---

# Nf Dev Workflow

Use this skill for repo orientation, command selection, and everyday
implementation hygiene.

## Module shape

Single Go module: `github.com/caian-org/nfe`. Binary entrypoint
`cmd/nfe/main.go`. All domain code lives under `internal/`:

- `internal/cli/` — Cobra command tree. `root.go` declares the version
  symbols injected by goreleaser ldflags.
- `internal/config/` — TOML configuration loader, defaults, validation.
- `internal/nota/` — TOML input schema for the emission flow.
- `internal/abrasf/` — ABRASF v2.04 builders, `Dec1`/`Dec2` types,
  response parsing, golden XML tests.
- `internal/xmlsig/` — Enveloped XMLDSig signer + PKCS#12 A1 loader.
- `internal/soap/` — SOAP 1.1 client + WSDL auto-discovery.
- `internal/service/` — `build → sign → send → parse` orchestration used
  by the CLI.
- `internal/render/` — Human and JSON renderers.
- `internal/validation/`, `internal/logging/` — small helpers.

Test fixtures: `testdata/config_full.toml`, `testdata/config_no_cert.toml`,
`testdata/nota_minimal.toml`. Golden XML files:
`testdata/golden/*.xml`.

## Command surface

Standard Go toolchain via `.justfile`:

```bash
just build         # produces bin/nfe
just test          # go test ./...
just test-race     # go test ./... -race
just cover         # coverage profile + per-function totals
just lint          # go vet ./... (CI also runs golangci-lint)
just tidy          # go mod tidy
just clean         # remove bin/ and coverage.out
just run ...        # build, then run bin/nfe with the given args
```

CLI surface (see `internal/cli/`):

- `nfe init [path]` — scaffold project layout (defaults to `~/.nfews`).
- `nfe status` / `nfe env <homologacao|producao>` — read-only inspection
  and active-environment switch.
- `nfe query --numero N` or `--data-inicial AAAA-MM-DD --data-final
  AAAA-MM-DD` — NFS-e lookup.
- `nfe emit input.toml [--dry-run]` — emit a new NFS-e.
- `nfe cancel --numero N --codigo C` — cancel an authorized NFS-e.

Every subcommand respects the global `-c/--config` flag (default
`./config.toml`) and the `--json` flag for machine-readable output.

## Validation lanes

Smallest useful check first.

- After a small code change: `just test`.
- Before opening a PR: `just test-race` plus `just lint` (CI runs the
  same plus golangci-lint).
- After touching XML builders: `go test ./internal/abrasf/... -update`
  to regenerate goldens, then read the diff carefully, then
  `just test-race` again.
- After touching SOAP/WSDL discovery: `go test ./internal/soap/...`.
- After touching the signer: `go test ./internal/xmlsig/...`.

## Implementation rules

- Idiomatic Go ≥ 1.26. `gofmt`/`goimports` via golangci-lint.
- Error wrapping: prefix in English (`pkg:`), body in pt-BR.
- Code comments and package doc strings in English. User-facing strings
  (CLI help, error message bodies, render labels, README) in pt-BR.
- Never reorder fields in `internal/abrasf/types.go` without regenerating
  goldens — field order maps directly to XML element order.

## The `ws/` rule

`ws/` is the user's local certificate + live configuration. It is
gitignored. Treat it as read-only personal data:

- Do not edit any file under `ws/`.
- Smoke tests against the real WS stay read-only (`status`, `query`).
  Never run `emit` or `cancel` against the user's real configuration.

## Commit hygiene

- Conventional commits (`feat:`, `fix:`, `chore:`, `docs:`, `test:`,
  `refactor:`).
- Keep commits focused; split schema/builder changes, transport changes,
  CLI changes, and test/data updates into separate commits when they
  evolve at different rates.
- After regenerating goldens, the golden delta belongs in the same
  commit as the builder change that caused it — never separately.
- CI runs on PR to `master`. Release via tag `v*` triggers goreleaser.
