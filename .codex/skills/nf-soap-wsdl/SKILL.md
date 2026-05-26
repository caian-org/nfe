---
name: nf-soap-wsdl
description: SOAP 1.1 transport and WSDL auto-discovery for nf. Use when changing the SOAP client, the envelope shape, mTLS plumbing, or anything around how endpoint URLs and SOAPAction values are discovered.
---

# Nf SOAP + WSDL

Use this skill before touching `internal/soap/`.

## Envelope shape

The SOAP 1.1 envelope built in `Client.buildEnvelope` is:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope
    xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"
    xmlns:tns="<methodNamespace>">
  <soap:Body>
    <tns:<action><requestSuffix>>
      <nfseCabecMsg><![CDATA[<cabecalho versao="2.04">...</cabecalho>]]></nfseCabecMsg>
      <nfseDadosMsg><![CDATA[<signed dados>]]></nfseDadosMsg>
    </tns:<action><requestSuffix>>
  </soap:Body>
</soap:Envelope>
```

Three things vary by deployment:

- `methodNamespace` — the xmlns of the method wrapper. Discovered from
  the WSDL's `<definitions targetNamespace=...>` attribute, overridable
  via `Options.MethodNamespace`.
- `<action><requestSuffix>` — the body wrapper element name. Defaults
  to `<action>Request` (e.g. `<ConsultarNfseServicoPrestadoRequest>`).
  Configurable via `Options.RequestSuffix`.
- `SOAPAction` HTTP header — discovered from the WSDL's
  `<soap:operation soapAction="..."/>` per operation. Defaults to the
  bare operation name when no WSDL is fetched.

`Cabecalho` is the fixed ABRASF 2.04 header constant declared in
`internal/soap/client.go`.

## WSDL auto-discovery quirks

The `internal/soap/wsdl.go` parser handles the ABRASF WSDLs in the
wild, which have four practical quirks:

1. **The endpoint host differs from the WSDL host.** Most ABRASF
   deployments publish the WSDL at `https://<env>.<municipio>/.../nfs?wsdl`
   but the `<soap:address location="..."/>` inside points at a different
   host (the actual service backend). Always trust the `address`, never
   strip `?wsdl` from the WSDL URL.
2. **The body element name is `<Method>Request`**, not `<Method>`.
   That is the `RequestSuffix` default. Some homologation environments
   accept either, production typically only accepts the suffixed form.
3. **The namespace comes from `targetNamespace`**, not from the
   endpoint URL or from `<service>`. Reading `targetNamespace` from the
   `<definitions>` root is the only reliable source.
4. **SOAPAction format is `nfs#<Method>`** in the WSDLs the parser has
   seen (e.g. `nfs#ConsultarNfseServicoPrestado`). The parser records
   them per operation in `WSDLInfo.SoapActions`.

`FetchWSDL` honours the supplied `*tls.Config` so the same mTLS
certificate is reused for both the WSDL fetch and the subsequent SOAP
calls. The parser is namespace-prefix-agnostic — it tolerates both
`soap:` and `wsdl:` prefix styles.

## mTLS + basic auth

`NewClient` builds an `*http.Client` with a `tls.Config` that includes
the user's PKCS#12 certificate as a client certificate (mTLS). When
`Options.BasicAuth` is set, `req.SetBasicAuth` is called on top of
mTLS — the two combine, neither replaces the other.

`Options.InsecureSkipVerify` is exposed because some homologation
environments use self-signed certs; production should never use it.
The flag mirrors `rejectUnauthorized: false` from the JS original.

## Response parsing

`ExtractBody(envelope)` decodes only the outer SOAP envelope and
returns the inner XML inside `<soap:Body>`. Downstream parsing in
`internal/abrasf/response.go` operates on those bytes.

SOAP faults are surfaced with the body intact and an error of the form
`soap: servidor retornou HTTP NNN`. Callers (in `internal/service/`)
inspect the body when faults arrive to extract `<Fault>` detail.

## Lazy client construction

`internal/service/service.go` constructs the SOAP client lazily so
`emit --dry-run` (which only builds + signs XML) works fully offline
without ever touching the network. Preserve that semantics — any
change to `NewClient` that triggers a network call eagerly will break
the dry-run path.

## Common pitfalls

- Using the WSDL URL host as the endpoint → 500s from the production
  backend.
- Forgetting the `Request` suffix → 500s or empty bodies.
- Sending `SOAPAction` without quotes → some servers reject the header.
  `soapActionFor` already wraps the value; do not unwrap it.
- Skipping mTLS in tests → `internal/soap/client_test.go` uses
  `net/http/httptest` with a stub TLS config; mirror that pattern in
  new tests.
