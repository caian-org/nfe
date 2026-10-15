---
name: nf-dps-nacional
description: National-layout DPS XML and the SIGCORP "WS Padrão Nacional" REST transport used by Franco da Rocha. Use when changing internal/dps, internal/sigcorp, the national branch of internal/service, or the dps goldens under testdata/golden.
---

# Nf DPS Nacional

Use this skill before touching `internal/dps/`, `internal/sigcorp/`,
`internal/service/emit_nacional.go`, or `testdata/golden/dps_*.xml`.

## When this path is active

`config.Config.Nacional != nil` (TOML section `[nacional]`). `service.Emit`,
`EmitDryRun`, `Query`, `QueryByRPS` and `Cancel` dispatch on
`cfg.IsNacional()`. `nfe init` scaffolds this mode. Without `[nacional]`
the ABRASF/SOAP path (`nf-abrasf-xml`, `nf-soap-wsdl`) is used.

## Wire format

`internal/dps/types.go` mirrors the Sistema Nacional NFS-e XSD v1.01
(`docs/vendor/franco-da-rocha/nacional/xsd-1.01/`). Invariants:

1. Struct field order = XSD sequence order. Reordering breaks the schema.
2. Element names are case-sensitive and mostly lowerCamel (`infDPS`,
   `cTribNac`, `vServ`); the IBSCBS leaves `CST`, `CNPJ`, `CEP`, `IM` are
   upper-case.
3. Decimals go through `dps.Dec2` (two fractional digits, matches
   `TSDec15V2`/`TSDec1V2` patterns). Never marshal bare `float64`.
4. `infDPS@Id` is `ID(cLocEmi, cnpj, serie, nDPS)` = `"DPS"` + 42 digits.
   `serie` travels without leading zeros but is zero-padded to 5 in the Id;
   `nDPS` is zero-padded to 15.
5. Optional groups are pointers with `omitempty` (`toma`, `end`, `infoCompl`,
   `pAliq`, `IBSCBS`). An empty struct must never serialize as an empty tag.

`Build(cfg, in, now)` holds the mapping rules (see
`docs/franco-da-rocha.md` → "XML"); `Marshal` prepends the XML header.
Signing uses `xmlsig.Signer.SignSibling(xml, "infDPS", id)` so `Signature`
lands as the second child of `DPS`, as the XSD declares.

`ParseNFSe` reads the approved NFS-e (`infNFSe@Id` minus the `NFS` prefix is
the 50-digit access key, `nNFSe`, `dhProc`, `vServ`, tomador name).

## Transport (`internal/sigcorp`)

- `POST {base}/v1/nfse/recepcao`: body = DPS XML, `Content-Type:
  application/xml`, headers `Authorization: Basic IM:senha`,
  `X-NFSe-Prestador-CNPJ` (digits), `X-NFSe-nNFSe`. `202` → `{protocolo}`.
- `GET {base}/v1/nfse/consulta/{protocolo}` → JSON with `status` in
  `em_processamento | rejeitado | recusada | erro_interno | aprovado`,
  plus `nfse` (XML string) and `linkImpressao` when approved.
- `ParseConsulta` is deliberately tolerant about the rejection-reason keys
  (`motivos`, `erros`, `mensagem`, …) because the vendor manual does not fix
  that shape. Keep it tolerant; add keys, do not narrow.
- Errors: `*HTTPError` (4xx, pt-BR message with the platform detail) and
  `*UnavailableError` (DNS, dial, 5xx). The service turns `HTTPError` into
  `MessagesError` (rendered, exit 0) and lets `UnavailableError` bubble
  (exit 1). `WaitConfirmacao` keeps polling through `UnavailableError`.

## Counters

`emitNacional` bumps `ProximoNumeroRPS` and `ProximoNumeroNFSe` as soon as
the platform answers `202`; the CLI saves the config **before** polling.
A later `rejeitado`/`recusada` does not roll back — the platform refuses a
reused `nNFSe`. Dry-run never touches counters or the network.

## Golden workflow

```bash
go test ./internal/dps/... -update   # regenerates dps_minimal.xml and dps_ibscbs.xml
go test ./internal/abrasf/...        # must stay green without -update
just test-race
```

Read the golden diff; every byte change must map to an intentional rule.

## Known gaps (document, do not hide)

- No cancel / query-by-number in the SIGCORP API.
- `tribFed/piscofins` is fixed at `CST=00`, `tpRetPisCofins=0` (PIS/COFINS/CSLL
  not withheld, no rates or base). Withholding is not configurable yet.
- ME/EPP prestadores must not send `indTotTrib`; the builder always sends 0.
- `IBSCBS/valores` in XSD v1.01 has no monetary fields; `[ibscbs]` rates are
  config documentation only.
- Whether the platform requires a signed DPS is confirmed in QA
  (`qa-webservice.meumunicipio.online`), never in production.
