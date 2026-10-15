## Port legado TypeScript -> Go

Este projeto Go foi portado a partir de
um repositório privado. A implementacao Go nao tenta portar todo o
escopo antigo; o README atual define o MVP como `init`, `status`, `env`,
`emit`, `query` e `cancel`.

## Comportamentos preservados

- Emissao de `GerarNfseEnvio` com RPS unico.
- Serie `00000` no XML de emissao, por convencao SIGISS usada pelo legado.
- SOAP com `nfseCabecMsg` e `nfseDadosMsg` dentro do wrapper `<Operacao>Request`.
- Assinatura XMLDSig enveloped em `InfDeclaracaoPrestacaoServico`.
- Defaults de CNAE `0000000`, pais `1058`, regime especial `1`,
  optante simples `2` e incentivo fiscal `2`.

## Divergencias intencionais

- O Go confia no WSDL real e usa SOAP 1.1, porque os WSDLs de Franco da Rocha
  declaram `soap:binding` SOAP 1.1. A documentacao legada que fala em SOAP 1.2
  esta desatualizada para esse provedor.
- O Go descobre `soap:address`, `targetNamespace` e `soapAction` pelo WSDL em
  vez de hardcodar apenas hosts.
- O Go usa certificado PKCS#12 A1 para assinatura e transporte via tipos nativos,
  em vez do arranjo Node/soap do app antigo.

## Bugs corrigidos no Go

- `ValorIss`: o TypeScript acabava emitindo zero em todos os ramos. No perfil
  atual nao optante pelo Simples, o Go emite o valor calculado.
- `observacoes`: o TypeScript anexava observacoes em `Discriminacao`; o Go agora
  preserva esse texto no campo obrigatorio e tambem mantem
  `InformacoesComplementares`.
- Wrappers de resposta: o Go aceita `outputXML`, `return`, `result` e `xml`,
  alinhando a compatibilidade cross-provider do helper TypeScript.
- Links: o Go aceita aliases comuns de URL, incluindo `LinkNota` e
  `LinkImpressao`. Franco da Rocha ABRASF normalmente nao retorna URL.

## Fora do escopo atual

Estas partes aparecem no app TypeScript ou nas referencias, mas nao fazem parte
do MVP Go atual:

- emissao em lote;
- consulta de lote;
- DANFSe/PDF;
- `ConsultarNfseFaixa`;
- `ConsultarNfseServicoTomado`;
- substituicao de NFS-e;
- uso completo de correlacao NBS (a correlacao item LC 116 x NBS x cIndOp x
  cClassTrib vem da planilha municipal e e informada no config/nota).

O layout nacional (DPS) e o grupo IBS/CBS estao implementados em
`internal/dps` para o WS Padrao Nacional da SIGCORP.

Qualquer documentacao importada do legado sobre esses topicos deve ficar
marcada como referencia ou trabalho futuro ate existir implementacao Go.
