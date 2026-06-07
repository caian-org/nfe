---
name: nf-abrasf-xml
description: ABRASF v2.04 XML emission rules for nf. Use when changing builders, decimal formatting, struct ordering, or golden XML fixtures under internal/abrasf and testdata/golden.
---

# Nf ABRASF XML

Use this skill before touching anything in `internal/abrasf/` or under
`testdata/golden/`.

## Wire format contract

Every builder marshals through `encoding/xml`. Two invariants are
load-bearing:

1. **Field order in structs determines XML element order.** ABRASF's XSD
   is strict about ordering; reordering a field silently breaks
   interop. The authoritative element layout lives in
   `internal/abrasf/types.go` — see `Valores`, `Servico`,
   `InfDeclaracaoPrestacaoServico`, and `ConsultarNfseServicoPrestadoEnvio`.
2. **Decimal precision must match the JS original byte-for-byte.** All
   monetary fields use `Dec2`, alíquota uses `Dec1` (one fractional
   digit; ABRASF rejects `5.00` where `5.0` is expected). The
   `MarshalXML` implementations in `internal/abrasf/decimal.go` are the
   contract; do not bypass them by using bare `float64`.

## Top-level envelopes

Builders return Go structs which marshal to one of three top-level
envelopes:

- `GerarNfseEnvio` — emission. Inner signed element is
  `InfDeclaracaoPrestacaoServico` (carries the `Id` attribute referenced
  by the XMLDSig `Reference`). In the 2025 municipal schema used by
  Franco da Rocha, the `<Signature>` block is emitted as a sibling after
  that `Inf...` inside `GerarNfseEnvio → Rps`, not as a child of `Inf...`.
- `ConsultarNfseServicoPrestadoEnvio` — read-only query. Same struct
  shape for both number-based and date-range lookups; `NumeroNfse` and
  `PeriodoCompetencia` are `omitempty`. `Pagina` is required.
- `CancelarNfseEnvio` — cancellation. Inner signed element is
  `InfPedidoCancelamento`. Container chain:
  `CancelarNfseEnvio → Pedido (PedidoContainer) → InfPedidoCancelamento`.

The `Xmlns` attribute is declared explicitly on each envelope so Go's
XML encoder doesn't inject a prefixed namespace declaration.

## Optional / discriminated fields

- `CpfCnpj` carries `CNPJ` and `CPF` with `omitempty` — exactly one of
  the two is emitted. Validators in `internal/nota/input.go` enforce
  the discriminated-union semantics before the builder runs.
- `TomadorServico` is fully optional; when present it may omit
  `IdentificacaoTomador` for foreign individuals without local
  documents (but the validator refuses notas without any tomador
  identification).
- `Valores.ValorIss` and `Valores.Aliquota` are pointer types so they
  can be absent when ISS is retained at source.

## Golden-file workflow

XML output is regression-tested by byte-for-byte comparison against
files in `testdata/golden/`. The test harness in
`internal/abrasf/abrasf_test.go` supports a `-update` flag:

```bash
go test ./internal/abrasf/... -update
```

Use this after any intentional change to the builders. Then:

1. Inspect the diff under `testdata/golden/` carefully — every byte
   change should be explainable.
2. Commit the builder change and the regenerated golden together. Never
   in separate commits.
3. Re-run `just test-race` to make sure the rest of the suite still
   passes.

## Common pitfalls

- Adding a struct field without `omitempty` when the underlying XSD
  treats it as optional → empty `<Foo></Foo>` element in the wire and
  silent rejection by some municipalities.
- Reordering fields "for readability" → element-order violation.
- Using `float64` in a new struct instead of `Dec1`/`Dec2` → drift from
  the JS original's `toFixed` semantics.
- Forgetting the `XMLName xml.Name \`xml:"..."\`` tag on a struct that
  needs a different XML element name than its Go identifier.

## Response parsing

Replies are parsed in `internal/abrasf/response.go`. The SOAP envelope
is stripped by `soap.ExtractBody`; what remains is the ABRASF
response (e.g. `<ConsultarNfseServicoPrestadoResposta>` /
`<GerarNfseResposta>` / `<CancelarNfseResposta>` or a
`<ListaMensagemRetorno>` error block). Response tests live next to the
builder tests; reuse those fixtures when adding new error paths.
