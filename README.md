[![CI][ci-shield]][ci-url]
[![Release][rel-shield]][rel-url]
[![GitHub tag][tag-shield]][tag-url]

# `nfe`

> CLI para emissão de NFS-e (Nota Fiscal de Serviço Eletrônica) padrão
> [ABRASF v2.04][abrasf]

`nfe` é um cliente de linha de comando para emitir, consultar e cancelar
NFS-e conforme o padrão ABRASF v2.04, usado por muitas prefeituras
brasileiras.

[ci-shield]: https://img.shields.io/github/actions/workflow/status/caian-org/nfe/ci.yml?label=ci&logo=github&style=flat-square
[ci-url]: https://github.com/caian-org/nfe/actions/workflows/ci.yml
[rel-shield]: https://img.shields.io/github/actions/workflow/status/caian-org/nfe/release.yml?label=release&logo=github&style=flat-square
[rel-url]: https://github.com/caian-org/nfe/actions/workflows/release.yml
[tag-shield]: https://img.shields.io/github/tag/caian-org/nfe.svg?logo=git&logoColor=FFF&style=flat-square
[tag-url]: https://github.com/caian-org/nfe/releases

[abrasf]: https://abrasf.org.br

## Status

MVP funcional cobrindo cinco comandos: `init`, `emit`, `query`, `cancel`,
`env` e `status`. Emissão em lote e geração de DANFSe (PDF) estão fora do
escopo.

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
./bin/nfe init                                  # cria ~/.nfews
# Edite ~/.nfews/config.toml com os dados do seu prestador, credenciais e os
# endpoints WSDL do seu município. Os comandos abaixo usam ~/.nfews/config.toml
# por padrão; use -c para apontar para outro caminho.
./bin/nfe status
./bin/nfe emit ~/.nfews/example-nota.toml --dry-run    # valida sem chamar o WS
./bin/nfe emit ~/.nfews/example-nota.toml              # emite de verdade
./bin/nfe query --numero 42
./bin/nfe cancel --numero 42 --codigo 1
```

## Comandos

| Comando | Função |
|---|---|
| `init [caminho]` | Cria a estrutura do projeto: `config.toml`, `example-nota.toml`, `README.md`. Padrão `~/.nfews`. |
| `emit <arquivo>` | Emite uma NFS-e a partir de um TOML. `--dry-run` gera e assina sem chamar o WS. `--wait-timeout` ajusta o timeout (padrão 60s). |
| `query` | Consulta NFS-e por `--numero` ou por `--data-inicial`/`--data-final`. |
| `cancel` | Cancela uma NFS-e autorizada: exige `--numero` e `--codigo` (1=erro emissão, 2=serviço não prestado, 3=erro processamento, 4=duplicidade). |
| `env <homologacao\|producao>` | Alterna o ambiente ativo. |
| `status` | Mostra a configuração ativa. |

### Flags globais

| Flag | Significado |
|---|---|
| `-c, --config` | Caminho do arquivo de configuração (padrão `~/.nfews/config.toml`). |
| `--json` | Emite saída JSON ao invés de texto humano. |

## Configuração (TOML)

```toml
ambiente = "homologacao"

[soap]
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
serie_rps          = "A"
proximo_numero_rps = 1
codigo_municipio   = "1234567"
aliquota_iss       = 5.0
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

O cliente descobre o endpoint SOAP, o namespace e o `SOAPAction` por
operação a partir do WSDL informado em `[soap]` (espelha o comportamento do
módulo `soap` do Node usado pelas implementações em TypeScript).

## Arquitetura

```
cmd/nfe                  -> main pequeno: monta root cobra, executa, sai
internal/cli             -> comandos cobra (init, emit, query, cancel, env, status)
internal/config          -> load/save TOML, expansão de env vars, validação
internal/nota            -> tipo de entrada TOML da nota e validação
internal/abrasf          -> tipos XML, builders, parser de resposta
internal/xmlsig          -> loader PKCS#12 + XMLDSig (RSA-SHA1, C14N inclusivo)
internal/soap            -> cliente SOAP 1.1 com mTLS + basic auth + auto-discovery do WSDL
internal/service         -> orquestra Emit/Query/Cancel
internal/render          -> renderizadores humano (tabelas coloridas) e JSON
internal/logging         -> factory de slog handler (sem emojis, nunca em stdout)
testdata/golden          -> envelopes XML comitados usados pelos testes do abrasf
```

A ordem dos campos em `internal/abrasf/types.go` segue o XSD do ABRASF —
reordenar campos produz XML que o WS rejeita.

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
```

Inspecione o diff antes de comitar — mudanças nos goldens são mudanças no
formato wire.

## Smoke contra um WS real

1. Coloque seu certificado A1 (`.p12` ou `.pfx`) em disco.
2. Exporte as variáveis referenciadas no `config.toml`:
   ```sh
   export NFE_USUARIO=...
   export NFE_SENHA=...
   export NFE_CERT_PATH=/caminho/para/cert.pfx
   export NFE_CERT_PASS=...
   ```
3. Aponte `[soap]` para o WSDL de homologação do seu município.
4. Execute:
   ```sh
   ./bin/nfe -c config.toml status                 # confere a config
   ./bin/nfe -c config.toml emit nota.toml --dry-run
   ./bin/nfe -c config.toml emit nota.toml
   ./bin/nfe -c config.toml query --numero <retornado>
   ./bin/nfe -c config.toml cancel --numero <retornado> --codigo 1
   ```

Se o WS rejeitar com um erro de regra de negócio, a CLI imprime as entradas
de `MensagemRetorno` (código + mensagem + correção) e sai com código 0. Erros
de rede, TLS ou de configuração saem com código diferente de zero.

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
