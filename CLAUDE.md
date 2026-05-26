# Claude Notes

Pointer doc for Claude Code agents working in `nf`.

**Read `AGENTS.md` first** — it is the canonical instruction layer. This file
covers only what is specific to Claude Code or to quick orientation.

## Bootstrap

1. `AGENTS.md` — canonical repository rules, layout, commands, testing, and
   the agent-specific constraints (read-only `ws/`, no live writes).
2. `README.md` — user-facing pt-BR documentation. Useful for terminology
   and the operator's mental model.
3. Relevant `.codex/skills/*/SKILL.md` for the subsystem being changed.

`AGENTS.md` is authoritative. If `CLAUDE.md` disagrees with it, follow
`AGENTS.md` and reconcile this file in the same change set.

## Claude Code specifics

- Skills are **never duplicated**: the canonical home is `.codex/skills/`.
  Claude Code reads them from that path; do not create copies under
  `.claude/skills/`.
- Specialist subagents live in both `.claude/agents/` (Markdown +
  front-matter, consumed by Claude Code) and `.codex/agents/` (TOML,
  consumed by Codex). Keep both copies in sync after editing — content is
  identical, only the serialization format differs.
- `.claude/settings.json` holds Claude Code project settings. Hooks are
  currently empty; if added later, keep them fail-open.
- README, CLI help, and error message bodies stay in pt-BR; agent-facing
  prose (AGENTS, CLAUDE, SKILL.md, agent prompts) and code comments stay
  in English. Preserve that split.

## Quick reference

Command surface:

- `just build` / `just test` / `just test-race` / `just cover` / `just lint`
  / `just tidy`.
- `./bin/nfe init [path]` — scaffold a project (defaults to `~/.nfews`).
- `./bin/nfe -c <path>/config.toml status` — show config summary.
- `./bin/nfe -c <path>/config.toml env homologacao|producao` — switch the
  active environment.
- `./bin/nfe -c <path>/config.toml query --numero N` or
  `--data-inicial AAAA-MM-DD --data-final AAAA-MM-DD` — read-only NFS-e
  lookup.
- `./bin/nfe -c <path>/config.toml emit input.toml` — emit a new NFS-e
  (write operation: never run against real `ws/`).
- `./bin/nfe -c <path>/config.toml cancel --numero N --codigo C` — cancel
  an authorized NFS-e (write operation).

Skills (canonical, read from `.codex/skills/`):

- `.codex/skills/nf-dev-workflow/SKILL.md`
- `.codex/skills/nf-abrasf-xml/SKILL.md`
- `.codex/skills/nf-xmldsig/SKILL.md`
- `.codex/skills/nf-soap-wsdl/SKILL.md`
- `.codex/skills/nf-cli-translation/SKILL.md`

Subagents (`.claude/agents/<name>.md` and `.codex/agents/<name>.toml`):

- `nf-architect` — cross-package design questions across
  `internal/{abrasf,xmlsig,soap,service}`.
- `nf-abrasf-xml-reviewer` — XML structure and signature placement audit.
- `nf-test-runner` — runs `just test-race` / `just cover` and reports.
- `nf-pt-br-translator` — pt-BR voice and consistency audit across
  user-facing strings.
