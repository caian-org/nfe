---
name: nf-xmldsig
description: Enveloped XMLDSig pipeline for nf. Use when changing the signer, the PKCS#12 loader, or anything that affects how the <Signature> block is computed and attached to ABRASF envelopes.
---

# Nf XMLDSig

Use this skill before touching `internal/xmlsig/`.

## Algorithm suite

ABRASF v2.04 mandates an exact algorithm suite:

- Digest: SHA-1.
- Signature: RSA-SHA1.
- Canonicalization (both `SignatureMethod` C14N and the inner
  `Transform`): `http://www.w3.org/TR/2001/REC-xml-c14n-20010315`
  (C14N 1.0 inclusive).
- `<Signature>` element carries `xmlns="..."` directly, **without** a
  `ds:` (or any other) namespace prefix.

These are baked into `NewSigner` in `internal/xmlsig/signer.go`. Do not
override unless ABRASF itself changes.

## Signing pipeline

`Signer.Sign(xmlBytes, targetName, targetID)` does the following:

1. Parses the XML with `etree`.
2. Depth-first searches for the element whose tag matches `targetName`
   and (if `targetID` is non-empty) whose `Id` attribute matches.
3. Asserts the target has a real parent element. The document root by
   itself has no parent to receive the signature — that case errors out
   with `xmlsig: elemento alvo não tem pai para receber a assinatura`.
4. Calls `goxmldsig.SignEnveloped(target)`. The library returns a copy
   of the target with `<Signature>` appended as its last child. The
   `Reference URI` becomes `#<Id>` when the target carries an `Id`
   attribute (the standard ABRASF case) or empty otherwise.
5. Swaps the original target for the signed copy at the same position
   in the parent so document structure is preserved.

Signed targets:

- For `GerarNfse`: target is `InfDeclaracaoPrestacaoServico`, with `Id`
  set on the struct. The signature ends up as the last child of the
  `InfDeclaracaoPrestacaoServico` element; its enclosing `<Rps>`
  container is untouched.
- For `CancelarNfse`: target is `InfPedidoCancelamento`. The `Id`
  attribute is `omitempty` in the Go struct but the canonical
  cancellation flow sets it.

`ConsultarNfse` is not signed.

## Certificate loading (PKCS#12 A1)

`internal/xmlsig/pkcs12.go` decodes the user's PFX/P12 file:

- Reads bytes from disk.
- Calls `pkcs12.Decode(pfxBytes, password)` from
  `software.sslmate.com/src/go-pkcs12`.
- Asserts the private key is `*rsa.PrivateKey`. Non-RSA keys (rare A3
  hardware tokens, EC certs) are rejected with
  `chave privada do PFX não é RSA`.

Returned tuple `(*x509.Certificate, *rsa.PrivateKey)` is what
`NewSigner` consumes.

## Path resolution

`xmlsig.LoadPFX` receives an already-resolved absolute filesystem path
and calls `os.ReadFile` on it. The resolution logic lives one layer up
in `internal/config/config.go`:

- `config.Load` records the absolute directory of the config file in
  the private `Config.configDir` field.
- `Config.CertificatePath()` returns the cert path resolved against
  that directory when the TOML value is relative; absolute values pass
  through unchanged; nil cert returns `""`.
- `service.New` calls `opts.Config.CertificatePath()` before invoking
  `LoadPFX`.

This means the TOML on disk stays portable (`Save()` round-trips the
original relative path the user wrote) and the resolution is anchored
to the config file, not to the current working directory.

## Common pitfalls

- Calling `Sign` with the document root as the target → no parent to
  attach the signature; error surfaces correctly but is easy to misread
  as "element not found".
- Targeting a structure-equivalent element by name only when multiple
  elements share that name → always pass `targetID` for envelopes that
  carry an `Id` attribute.
- Mutating the document after signing → invalidates the digest.
- Mixing canonicalization variants (e.g. exclusive C14N) → ABRASF
  validators reject the signature.

## Validating a signed document by hand

A quick sanity check: run `xmllint --c14n` against the signed payload
and feed the canonicalized bytes through `openssl dgst -sha1` — the
digest should match the `<DigestValue>` block in the signature.
