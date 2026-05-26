# Repository Guidelines

## Project Structure & Module Organization

`nf` is a Go CLI for emitting, querying, and cancelling Brazilian NFS-e
documents that follow the ABRASF v2.04 standard. The repository is a single
Go module (`github.com/caian-org/nfe`); the binary entrypoint is
`cmd/nfe/main.go` and all domain logic lives under `internal/`.

- `internal/cli/` — Cobra commands (`init`, `env`, `status`, `emit`, `query`,
  `cancel`) and their tests. `root.go` declares the `Version`/`Commit`/
  `BuildDate` symbols injected by goreleaser ldflags.
- `internal/config/` — TOML configuration loader, defaults, and validation.
- `internal/abrasf/` — ABRASF v2.04 builders (`GerarNfse`,
  `ConsultarNfseServicoPrestado`, `CancelarNfse`), fixed-decimal types
  (`Dec1`/`Dec2`), response parsing, and the golden XML test suite.
- `internal/xmlsig/` — Enveloped XMLDSig signer (RSA-SHA1, C14N inclusive)
  and PKCS#12 A1 certificate loader.
- `internal/soap/` — SOAP 1.1 client, WSDL auto-discovery, and the mTLS +
  optional basic-auth transport plumbing.
- `internal/service/` — High-level orchestration (build → sign → send →
  parse) consumed by the CLI layer.
- `internal/nota/` — TOML input schema for the emission flow.
- `internal/render/` — Human and JSON renderers for CLI output.
- `internal/validation/`, `internal/logging/` — small helpers.
- `testdata/` — TOML fixtures (`config_full.toml`, `config_no_cert.toml`,
  `nota_minimal.toml`) and the golden XML files under `testdata/golden/`.

`bin/`, `coverage.out`, and the user-local `.nfews/` directory (created by
`nfe init`) are generated artefacts — never edit by hand.

## Build, Test, and Development Commands

Standard Go toolchain. Common targets:

- `make build` — builds `bin/nfe`.
- `make test` — runs `go test ./...`.
- `make test-race` — runs the suite with `-race`.
- `make cover` — coverage profile plus per-function totals.
- `make lint` — currently `go vet ./...`. CI additionally runs
  `golangci-lint` per `.golangci.yml`.
- `make tidy` — `go mod tidy`.
- `go test ./internal/abrasf/... -update` — regenerate the XML golden
  fixtures under `testdata/golden/`.

## Coding Style & Naming Conventions

- Idiomatic Go ≥ 1.26 with `gofmt` and `goimports`, enforced via
  golangci-lint.
- The struct field order in `internal/abrasf/types.go` is authoritative
  for XML element order. Never reorder without regenerating goldens.
- Decimal formatting goes through `Dec1`/`Dec2` (`internal/abrasf/decimal.go`)
  so the wire format matches the JS original byte-for-byte.
- Errors wrap with a package prefix in English followed by a Portuguese
  message body — e.g. `xmlsig: documento vazio`,
  `soap: falha ao montar envelope`. The prefix aids diagnosis; the body
  surfaces to the user via the CLI renderer.
- Code comments and package doc strings stay in English. README, CLI
  help, render labels (`OK:`, `ERRO:`, `dica:`), and error message bodies
  stay in pt-BR.

## Testing Guidelines

`testify` is used for assertions. XML builders are validated by byte-for-
byte golden file comparison in `internal/abrasf/abrasf_test.go` against
`testdata/golden/*.xml`. Run `make test-race` before opening a PR; the
race detector catches the goroutine-driven HTTP fixtures in
`internal/soap/client_test.go` and the lazy-init paths in
`internal/service/service.go`.

When changing builders, prefer regenerating goldens via `-update` and
re-reading the diff over hand-editing the XML.

## Commit & Pull Request Guidelines

Conventional commits (`feat:`, `fix:`, `chore:`, `docs:`, `test:`).
CI (`.github/workflows/ci.yml`) runs on every PR to `master` with vet +
race-enabled tests + golangci-lint. Releases are cut by pushing a tag
matching `v*`, which triggers `.github/workflows/release.yml` and
goreleaser (`.goreleaser.yaml`) to publish multi-platform archives.

## Agent-Specific Instructions

Three rules every agent must honour in this repo:

1. **Never run write-side operations** (`emit`, `cancel`) against the
   user's real `ws/` configuration. Live smoke tests stay read-only:
   `status`, `query`. The user has explicitly enforced this.
2. **Do not edit anything under `ws/`.** That directory is gitignored
   and contains the user's certificate and live config. Treat it as
   read-only personal data.
3. **Read the relevant `.codex/skills/*/SKILL.md` before non-trivial
   changes** in a domain. The skills are the canonical knowledge layer
   shared by Claude Code and Codex; updating them is part of the change
   when the underlying behaviour shifts.
