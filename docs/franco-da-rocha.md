# Franco da Rocha — WS Padrão Nacional (SIGCORP)

Notas operacionais para emitir NFS-e em Franco da Rocha/SP com o `nfe`.
Verificado em 2026-10-15.

## Como a prefeitura recebe notas

O município mantém o emissor próprio (SIGISS / MeuMunicipio, da SIGCORP) e
exige o **layout nacional** desde 2026-10-01. A integração por webservice é a
API REST "WS Padrão Nacional", documentada em:

- Portal ISS → "Integração WS Padrão Nacional":
  `https://francodarocha.meumunicipio.online/ISS/download/WS_Nacional.php`
- Manual Complementar de Integração — NFS-e, v1.01 de 01/09/2026:
  `https://sigcorp.com.br/manual-complementar-de-integracao-nfs-e/`
- Manual Complementar de Emissão (tela), v1.1 de 01/10/2026, publicado na
  mesma página do portal.

Textos em `docs/vendor/franco-da-rocha/nacional/sigcorp/`.

## Endpoints

| ambiente | base |
|---|---|
| produção | `https://webservice.meumunicipio.online` |
| QA / homologação | `https://qa-webservice.meumunicipio.online` |

- `POST /v1/nfse/recepcao` — corpo: XML da DPS (`Content-Type: application/xml`),
  até 2 MB. Resposta `202 {"protocolo": "…"}`; `400` XML/cabeçalho inválido,
  `401` credenciais, `403` IM do login diverge do prestador da DPS.
- `GET /v1/nfse/consulta/{protocolo}` — `{"status": …}` com
  `em_processamento`, `rejeitado` (motivos da plataforma), `recusada`
  (motivo da ADN), `erro_interno`, `aprovado` (`nfse` = XML da NFS-e no
  padrão ADN, `linkImpressao` = DANFSe).

Cabeçalhos em toda chamada: `Authorization: Basic base64(IM:senha)`,
`X-NFSe-Prestador-CNPJ` (só dígitos). Na recepção também `X-NFSe-nNFSe`,
o número da NFS-e escolhido pelo prestador (até 13 dígitos, único; repetido é
rejeitado). Não há certificado digital na autenticação.

O `nfe` guarda `proximo_numero_nfse` em `[configuracoes]` e o incrementa, junto
com `proximo_numero_rps`, assim que a recepção devolve `202`. Uma rejeição
posterior não devolve o número.

## XML

A DPS segue o XSD v1.01 do Sistema Nacional NFS-e
(`docs/vendor/franco-da-rocha/nacional/xsd-1.01/`), sem alterações da
SIGCORP. O que o `nfe` envia (`internal/dps`):

- `infDPS@Id = "DPS" + cLocEmi(7) + "2" + CNPJ(14) + série(5) + nDPS(15)`.
- `tpAmb` 1/2 conforme `ambiente`; `dhEmi` com offset local; `dCompet` = hoje;
  `tpEmit = 1`; `cLocEmi = configuracoes.codigo_municipio` (IBGE de Franco da
  Rocha: `3516408`). O QA da SIGCORP valida este código antes da autenticação
  e responde `400 Município não encontrado` para códigos desconhecidos.
- `prest`: CNPJ, IM, `regTrib{opSimpNac, regEspTrib}` de `[nacional]`.
- `toma`: CPF/CNPJ, `xNome`, endereço nacional completo, fone, e-mail.
- `serv`: `locPrest/cLocPrestacao` (`servico.local_prestacao` ou o município
  emissor), `cServ{cTribNac, xDescServ, cNBS}`, `infoCompl/xInfComp` com as
  observações.
- `valores`: `vServ`; `tribMun{tribISSQN=1, tpRetISSQN 1|2}`; `pAliq` apenas
  para Simples Nacional ou prestação fora do município (regra da plataforma,
  faixa 2–5%); `tribFed/piscofins{CST=00, tpRetPisCofins=0}` declara que
  PIS/COFINS/CSLL não são retidos e não informa alíquota nem base;
  `totTrib/indTotTrib=0`.
- `IBSCBS` só com `[ibscbs]` no config: `finNFSe=0`, `cIndOp`, `indDest=0`,
  `valores/trib/gIBSCBS{CST, cClassTrib}`. A plataforma não calcula IBS/CBS e
  exige `cNBS` junto. Prestadores do Simples não podem enviar o grupo.
- Assinatura: `Signature` enveloped (RSA-SHA1, C14N) como irmão de `infDPS`,
  quando há certificado e `nacional.assinar_dps` não está desligado. A API
  não exige certificado; a necessidade de assinar a DPS é confirmada em QA.

Exemplo de códigos para o item 01.01 com NBS 115022000, da planilha
`tabela_servicos_nacional_final.xlsx`: `cTribNac 010101`, `cIndOp 100301`,
`cClassTrib 000001`, `CST 000`, local de incidência = domicílio do adquirente.

## Limites

- Não há cancelamento nem consulta por número na API; `nfe cancel` e
  `nfe query --numero` respondem com erro explicativo no modo nacional.
  Cancelamento é feito pelo portal da prefeitura.
- Prestadores ME/EPP (`op_simp_nac = 3`) não podem enviar `indTotTrib`; o
  `nfe` hoje envia `indTotTrib=0` para todos os regimes.
- O grupo `IBSCBS/valores` do XSD v1.01 não tem campos monetários; as
  alíquotas de `[ibscbs]` ficam no config como referência.

## Webservice ABRASF (histórico)

Até 2026-09-30 a prefeitura publicava WSDLs ABRASF 2.04 em
`https://francodarocha.meumunicipio.online/abrasf/ws/nfs?wsdl` (produção) e
`https://testefrancodarocha.meumunicipio.online/abrasf/ws/nfs?wsdl`
(homologação), com `soap:address` em `abrasffrancodarocha.meumunicipio.online`
e `testefrancodarochaabrasf.meumunicipio.online`. Em 2026-10-05 esses hosts
saíram do DNS e os WSDLs passaram a responder `502 Proxy Error`. O código
SOAP em `internal/soap` e `internal/abrasf` permanece para outras
prefeituras; o `nfe` reporta esses casos como "webservice municipal
indisponível".
