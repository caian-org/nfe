# Franco da Rocha — material de referência

Documentos públicos usados para implementar o modo nacional do `nfe`
(`internal/dps`, `internal/sigcorp`). Notas operacionais em
`docs/franco-da-rocha.md`.

## `nacional/sigcorp/`

- `manual-complementar-integracao-v1.01.txt` — texto do "Manual Complementar
  de Integração — NFS-e" v1.01 (01/09/2026), SIGCORP. Endpoints, autenticação,
  cabeçalhos, situações da consulta e regras próprias da plataforma.
  Fonte: https://sigcorp.com.br/manual-complementar-de-integracao-nfs-e/
- `SIGCORP-Manual_Complementar_Emissao_NFS-e_v2.txt` — texto do manual de
  emissão em tela v1.1 (01/10/2026); a seção 7 mapeia campos da tela para a
  DPS. Fonte: portal ISS da prefeitura, menu "Integração WS Padrão Nacional"
  (`https://francodarocha.meumunicipio.online/ISS/download/WS_Nacional.php`).

## `nacional/`

- `xsd-1.01/` — esquemas XSD v1.01 do Sistema Nacional NFS-e (DPS, NFSe,
  tipos simples e complexos, eventos, CNC, xmldsig). Fonte:
  https://www.gov.br/nfse/pt-br/biblioteca/documentacao-tecnica/documentacao-atual
- `tabela_servicos_nacional.csv` — tabela municipal item LC 116 × NBS × cIndOp
  × cClassTrib × CST (exportada da planilha `tabela_servicos_nacional_final.xls`
  publicada no portal ISS da prefeitura, menu "Integração Web Service").

Anexos oficiais consultados e não copiados: Anexo I (leiaute DPS/NFS-e, com a
composição do `Id`), Anexo B (NBS × lista de serviços) e Anexo C (códigos
indicadores de operação), todos na mesma biblioteca do gov.br.
