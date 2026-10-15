[![CI][ci-shield]][ci-url]
[![Release][rel-shield]][rel-url]
[![GitHub tag][tag-shield]][tag-url]

# `nfe`

> CLI para emissão de NFS-e (Nota Fiscal de Serviço Eletrônica) no layout
> nacional via SIGCORP (Franco da Rocha) e no padrão [ABRASF v2.04][abrasf]

`nfe` é um cliente de linha de comando para emitir e consultar NFS-e. Em
Franco da Rocha ele monta a DPS do Sistema Nacional NFS-e e a envia ao
**WS Padrão Nacional da SIGCORP** (API REST). Para outras prefeituras,
continua falando SOAP com webservices ABRASF v2.04.

[ci-shield]: https://img.shields.io/github/actions/workflow/status/caian-org/nfe/ci.yml?label=ci&logo=github&style=flat-square
[ci-url]: https://github.com/caian-org/nfe/actions/workflows/ci.yml
[rel-shield]: https://img.shields.io/github/actions/workflow/status/caian-org/nfe/release.yml?label=release&logo=github&style=flat-square
[rel-url]: https://github.com/caian-org/nfe/actions/workflows/release.yml
[tag-shield]: https://img.shields.io/github/tag/caian-org/nfe.svg?logo=git&logoColor=FFF&style=flat-square
[tag-url]: https://github.com/caian-org/nfe/releases

[abrasf]: https://abrasf.org.br

## Status

Comandos: `init`, `emit`, `query`, `cancel`, `env` e `status`. O modo
nacional (SIGCORP) cobre `emit` e `query --protocolo`; cancelamento e
consulta por número só existem no modo ABRASF. Emissão em lote e geração de
DANFSe (PDF) estão fora do escopo — no modo nacional a prefeitura devolve o
link de impressão do DANFSe.

## Build

```sh
just build              # gera ./bin/nfe
./bin/nfe --version
```

Requer Go 1.21+ (desenvolvido contra 1.26).

### Via Docker

```sh
docker run --rm ghcr.io/caian-org/nfe:latest --help
```

## Início rápido

```sh
./bin/nfe init                                  # cria ~/.nfews já no modo nacional (SIGCORP)
# Edite ~/.nfews/config.toml com os dados do seu prestador, a senha do portal
# e os códigos tributários. As notas ficam em ~/.nfews/notas/.
# Os comandos abaixo usam ~/.nfews por padrão; use --workspace para outro caminho.
./bin/nfe status
./bin/nfe emit example --dry-run                # monta e assina a DPS sem chamar o WS
./bin/nfe emit example                          # envia, salva os contadores e aguarda a aprovação
./bin/nfe query --protocolo <protocolo>         # reconsulta uma emissão pendente
```

## Comandos

| Comando | Função |
|---|---|
| `init [caminho]` | Cria a estrutura do projeto: `config.toml`, `notas/example.toml`, `README.md`. Padrão `~/.nfews`. |
| `emit <nota>` | Emite uma NFS-e a partir de `notas/<nota>.toml` no workspace. `--dry-run` gera e assina sem chamar o WS. `--verbose` mostra o XML assinado e as respostas brutas. Por padrão aguarda a confirmação (protocolo no modo nacional, RPS no ABRASF); `--no-confirmation-wait` desliga essa espera. `--wait-timeout` ajusta o timeout da requisição. |
| `query` | Modo nacional: `--protocolo <p>` consulta a situação de uma emissão. Modo ABRASF: `--numero` ou `--data-inicial`/`--data-final`. |
| `cancel` | Apenas modo ABRASF. Cancela uma NFS-e autorizada: exige `--numero` e `--codigo` (1=erro emissão, 2=serviço não prestado, 3=erro processamento, 4=duplicidade). No modo nacional o comando informa que o cancelamento é feito pelo portal da prefeitura. |
| `env <homologacao\|producao>` | Alterna o ambiente ativo. |
| `status` | Mostra a configuração ativa. |

### Flags globais

| Flag | Significado |
|---|---|
| `-w, --workspace` | Diretório de trabalho com `config.toml` e `notas/` (padrão `~/.nfews`). |
| `--json` | Emite saída JSON ao invés de texto humano. |

## Franco da Rocha — WS Padrão Nacional (SIGCORP)

Desde 2026-10-01 a prefeitura de Franco da Rocha exige o layout nacional e
recebe as notas pela API REST da SIGCORP (o webservice SOAP/ABRASF antigo foi
desligado). O `nfe` monta a **DPS** (Declaração de Prestação de Serviço, XSD
v1.01, namespace `http://www.sped.fazenda.gov.br/nfse`), assina `infDPS` com
o certificado A1 quando houver, e:

1. `POST {url}/v1/nfse/recepcao` com o XML da DPS, autenticação HTTP Basic
   (inscrição municipal + senha do portal), `X-NFSe-Prestador-CNPJ` e
   `X-NFSe-nNFSe` (o número da NFS-e é escolhido pelo prestador);
2. recebe `202 {"protocolo": …}` e **salva os contadores** (`proximo_numero_rps`
   e `proximo_numero_nfse`) — a plataforma rejeita número de NFS-e repetido,
   então um número aceito nunca é reutilizado, mesmo que a nota seja rejeitada
   depois;
3. consulta `GET {url}/v1/nfse/consulta/{protocolo}` até `aprovado`,
   `rejeitado`, `recusada` ou `erro_interno`, dentro de `confirm_timeout`.
   Em `aprovado` mostra número, chave de acesso e link do DANFSe.

Hosts publicados pela prefeitura (`nfe init` já os escreve):

| ambiente | URL |
|---|---|
| producao | `https://webservice.meumunicipio.online` |
| homologacao | `https://qa-webservice.meumunicipio.online` |

Fonte: página "Integração WS Padrão Nacional" do portal ISS e o Manual
Complementar de Integração da SIGCORP v1.01 (01/09/2026); texto em
`docs/vendor/franco-da-rocha/nacional/sigcorp/`.

## Configuração (TOML)

```toml
ambiente = "homologacao"

[nacional]                     # presença desta seção liga o modo nacional
url_producao    = "https://webservice.meumunicipio.online"
url_homologacao = "https://qa-webservice.meumunicipio.online"
serie_dps       = "1"          # numérica, até 5 dígitos (faixa 1–49999 para aplicativo próprio)
versao_aplicacao = "nfe"       # verAplic da DPS
op_simp_nac     = 1            # 1 não optante, 2 MEI, 3 ME/EPP
reg_esp_trib    = 0            # 0 nenhum … 6, 9
assinar_dps     = true         # assina infDPS quando há certificado configurado

[ibscbs]                       # opcional; sem a seção a DPS sai sem o grupo IBSCBS
cst = "000"
codigo_classificacao_tributaria = "000001"
codigo_indicador_operacao       = "100301"
aliquota_ibs_uf        = 0.1   # percentuais do ano-teste 2026
aliquota_ibs_municipal = 0.0
aliquota_cbs           = 0.9

[soap]                         # usado apenas quando [nacional] está ausente
wsdl_homologacao = "https://teste.exemplo.com.br/abrasf/ws/nfs?wsdl"
wsdl_producao    = "https://producao.exemplo.com.br/abrasf/ws/nfs?wsdl"

[prestador]
cnpj = "00000000000000"
inscricao_municipal = "000000"
razao_social = "EMPRESA EXEMPLO LTDA"
nome_fantasia = "EXEMPLO"

[prestador.endereco]
endereco = "RUA EXEMPLO"
numero = "100"
bairro = "CENTRO"
codigo_municipio = "1234567"
uf = "SP"
cep = "01234000"

[prestador.contato]
telefone = "1100000000"
email = "contato@exemplo.com.br"

[autenticacao]
usuario = "${NFE_USUARIO}"
senha   = "${NFE_SENHA}"

[autenticacao.certificado]
path  = "${NFE_CERT_PATH}"   # caminho do seu A1 .pfx
senha = "${NFE_CERT_PASS}"

[configuracoes]
serie_rps           = "A"      # série do RPS no modo ABRASF
proximo_numero_rps  = 1        # nDPS no modo nacional / RPS no ABRASF
proximo_numero_nfse = 1        # número da NFS-e enviado em X-NFSe-nNFSe (modo nacional)
codigo_municipio    = "3516408" # IBGE do município emissor (cLocEmi); Franco da Rocha = 3516408
aliquota_iss        = 2.0
confirm_timeout     = "2m"
confirm_interval    = "5s"
```

No modo nacional a autenticação é `autenticacao.senha` (senha do portal da
prefeitura) com usuário `autenticacao.usuario` ou, quando vazio, a própria
`prestador.inscricao_municipal`. O certificado A1 continua opcional e serve
só para assinar a DPS.

Os códigos de `[ibscbs]` e `servico.codigo_tributacao_nacional` vêm da
planilha municipal `tabela_servicos_nacional_final.xlsx` (item LC 116 × NBS
× cIndOp × cClassTrib). Para o item 01.01 com NBS 115022000:
`codigo_tributacao_nacional = "010101"`, `cIndOp = 100301`,
`cClassTrib = 000001`, `CST = 000`. A plataforma não calcula IBS/CBS: o grupo
é opcional e, quando enviado, `cnbs` passa a ser obrigatório. Prestadores do
Simples Nacional não podem enviar o grupo. As alíquotas ficam no config para
documentação e para o grupo de valores quando o leiaute passar a exigi-lo —
no XSD v1.01 do DPS o grupo `IBSCBS/valores` só leva `trib/gIBSCBS`.

### Nota (TOML)

```toml
cnbs = "115022000"             # NBS de 9 dígitos; obrigatório com [ibscbs]
observacoes = "Projeto X"      # vira serv/infoCompl/xInfComp

[tomador]
cnpj = "00000000000000"
razao_social = "TOMADOR EXEMPLO LTDA"
email = "contato@tomador-exemplo.com.br"

[tomador.endereco]             # endereço completo: o ambiente nacional valida tudo
endereco = "RUA EXEMPLO"
numero = "100"
bairro = "CENTRO"
codigo_municipio = "3550308"
municipio = "SAO PAULO"        # informativo
uf = "SP"
cep = "01234000"

[servico]
discriminacao = "DESCRIÇÃO DO SERVIÇO PRESTADO"
valor_servicos = 1000.0
item_lista_servico = "0101"
codigo_tributacao_nacional = "010101"  # cTribNac, obrigatório no modo nacional
local_prestacao = "3516408"            # opcional; padrão = codigo_municipio do config
iss_retido = false
aliquota = 2.0                         # só vai na DPS (pAliq) para Simples ou prestação fora do município

[ibscbs]                       # opcional: sobrescreve os códigos do config nesta nota
cst = "000"
codigo_classificacao_tributaria = "000001"
codigo_indicador_operacao = "100301"
```

O `path` do certificado pode ser absoluto ou relativo. Quando relativo,
é interpretado a partir do diretório do `config.toml` (ex.: se o
`config.toml` está em `~/.nfews/` e `path = "./cert.pfx"`, o `nfe`
procura por `~/.nfews/cert.pfx` independente de onde o comando foi
executado).

Referências `${VAR}` e `$VAR` são expandidas contra as variáveis de ambiente
do processo. Variáveis não definidas permanecem como o literal `${VAR}` no
config, fazendo a validação detectar valores ausentes.

Após cada emissão bem-sucedida, `proximo_numero_rps` é incrementado e o
config é reescrito atomicamente (arquivo temporário + rename) — seu contador
sequencial sobrevive a crashes durante a escrita.

No modo nacional, o `emit` consulta o protocolo até a situação final ou até
`confirm_timeout`; se o prazo estourar, a saída traz o comando
`nfe query --protocolo <p>` para reconsultar. No modo ABRASF, quando o
`GerarNfse` responde de forma assíncrona, o `emit` consulta
`ConsultarNfsePorRps` da mesma forma. `confirm_interval` controla o intervalo
entre tentativas. Use `--no-confirmation-wait` para retornar logo após o
envio.

Durante a emissão, a CLI mostra o andamento em `stderr` (`nota`, `xml`,
`assinatura`, `prefeitura`, `confirmação`, `config`). Em TTY, esse progresso
é compacto e animado; em scripts é texto simples. Com `--json`, não há
progresso nem ANSI fora do JSON em `stdout`.

No modo ABRASF o cliente descobre o endpoint SOAP, o namespace e o
`SOAPAction` por operação a partir do WSDL informado em `[soap]`, ou usa
`soap.endpoint_*` diretamente quando configurado. Falhas de DNS, conexão e
gateway (502/503/504) são reportadas como "webservice indisponível" em ambos
os modos.

## Arquitetura

```
cmd/nfe                  -> main pequeno: monta root cobra, executa, sai
internal/cli             -> comandos cobra (init, emit, query, cancel, env, status)
internal/config          -> load/save TOML, expansão de env vars, validação
internal/nota            -> tipo de entrada TOML da nota e validação
internal/dps             -> DPS do layout nacional (tipos, builder, parser da NFS-e)
internal/sigcorp         -> cliente REST do WS Padrão Nacional (recepção + consulta)
internal/abrasf          -> tipos XML ABRASF 2.04, builders, parser de resposta
internal/xmlsig          -> loader PKCS#12 + XMLDSig (RSA-SHA1, C14N inclusivo)
internal/soap            -> cliente SOAP 1.1 com mTLS + basic auth + auto-discovery do WSDL
internal/service         -> orquestra Emit/Query/Cancel (despacha nacional × ABRASF)
internal/render          -> renderizadores humano (saída semântica) e JSON
internal/logging         -> factory de slog handler (sem emojis, nunca em stdout)
testdata/golden          -> XML comitado usado pelos testes de abrasf e dps
```

A ordem dos campos em `internal/dps/types.go` e `internal/abrasf/types.go`
segue os XSD — reordenar campos produz XML que o WS rejeita.

## Testes

```sh
just test               # suite completa
just test-race          # com -race
just cover              # relatório de cobertura
```

Goldens ficam em `testdata/golden/`. Para regenerar após uma mudança
intencional no XML:

```sh
go test ./internal/abrasf/... -update
go test ./internal/dps/... -update
```

Inspecione o diff antes de comitar — mudanças nos goldens são mudanças no
formato wire.

## Smoke contra um WS real

1. Exporte as variáveis referenciadas no `config.toml` (senha do portal e,
   se usar, certificado A1):
   ```sh
   export NFE_SENHA=...
   export NFE_CERT_PATH=/caminho/para/cert.pfx
   export NFE_CERT_PASS=...
   ```
2. Use `ambiente = "homologacao"` (QA da SIGCORP) e um workspace separado do
   de produção.
3. Execute:
   ```sh
   ./bin/nfe --workspace . status                 # confere ./config.toml
   ./bin/nfe --workspace . emit nota --dry-run --verbose   # mostra a DPS assinada
   ./bin/nfe --workspace . emit nota
   ./bin/nfe --workspace . query --protocolo <protocolo>
   ```

Se a plataforma rejeitar a nota (`rejeitado`/`recusada`, ou HTTP 400/401/403
na recepção), a CLI imprime os motivos e sai com código 0. Erros de rede,
indisponibilidade do webservice ou de configuração saem com código diferente
de zero. No modo ABRASF o fluxo é o mesmo com `query --numero` e `cancel`.

## Licença

Na medida do permitido por lei, [Caian Ertl][me] renunciou a __todos os
direitos autorais e direitos conexos a este trabalho__. No espírito de
_liberdade de informação_, você é encorajado a clonar, modificar, distribuir,
compartilhar ou fazer o que quiser com este projeto! [`^C ^V`][kopimi]

[![Licença][cc-shield]][cc-url]

[me]: https://github.com/upsetbit
[cc-shield]: https://forthebadge.com/images/badges/cc-0.svg
[cc-url]: http://creativecommons.org/publicdomain/zero/1.0
[kopimi]: https://kopimi.com
