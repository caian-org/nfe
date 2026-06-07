package cli

import (
	"bytes"
	"encoding/xml"
	"errors"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/caian-org/nfe/internal/abrasf"
	"github.com/caian-org/nfe/internal/config"
	"github.com/caian-org/nfe/internal/render"
	"github.com/caian-org/nfe/internal/service"
)

func newQueryCmd(gf *globalFlags) *cobra.Command {
	var (
		numero      string
		dataInicial string
		dataFinal   string
		timeout     time.Duration
	)
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Consulta NFS-e por número ou intervalo de datas",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if numero == "" && (dataInicial == "" || dataFinal == "") {
				return errors.New("informe --numero ou ambos --data-inicial e --data-final")
			}

			configPath, err := gf.configPath()
			if err != nil {
				return err
			}
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			svc, err := service.New(service.Options{Config: cfg})
			if err != nil {
				return err
			}
			ctx, cancel := contextWithTimeout(cmd, timeout)
			defer cancel()

			res, err := svc.Query(ctx, abrasf.ConsultaQuery{
				Numero:      numero,
				DataInicial: dataInicial,
				DataFinal:   dataFinal,
			})
			info := render.QueryInfo{}
			if res != nil {
				info.Mensagens = mapMensagens(res.Mensagens)
				info.Response = string(res.BodyInner)
				info.NFSes = parseQueriedNFSes(res.BodyInner)
			}
			var msgErr *service.MessagesError
			switch {
			case err == nil:
				info.Sucesso = true
			case errors.As(err, &msgErr):
				info.Sucesso = false
			default:
				return err
			}
			return gf.renderer(cmd).Query(info)
		},
	}
	cmd.Flags().StringVarP(&numero, "numero", "n", "", "número da NFS-e a consultar")
	cmd.Flags().StringVar(&dataInicial, "data-inicial", "", "data inicial (AAAA-MM-DD) para consulta por período")
	cmd.Flags().StringVar(&dataFinal, "data-final", "", "data final (AAAA-MM-DD) para consulta por período")
	cmd.Flags().DurationVar(&timeout, "wait-timeout", 0, "timeout da requisição (ex.: 30s, 2m)")
	return cmd
}

// parseQueriedNFSes extracts a flat list of NFS-e records from a query
// response payload. Tolerant of minor schema variations across municipalities.
//
// The shape we target (typical ABRASF v2.04 response):
//
//	CompNfse > Nfse > InfNfse > {
//	  Numero, CodigoVerificacao, DataEmissao,
//	  ValoresNfse > BaseCalculo,
//	  DeclaracaoPrestacaoServico > InfDeclaracaoPrestacaoServico >
//	    Servico > Valores > ValorServicos,
//	    TomadorServico > RazaoSocial
//	}
func parseQueriedNFSes(body []byte) []render.QueriedNFSe {
	if len(body) == 0 {
		return nil
	}
	type valoresInner struct {
		ValorServicos string `xml:"ValorServicos"`
	}
	type servico struct {
		Valores valoresInner `xml:"Valores"`
	}
	type tomador struct {
		RazaoSocial string `xml:"RazaoSocial"`
	}
	type infDec struct {
		Servico        servico `xml:"Servico"`
		TomadorServico tomador `xml:"TomadorServico"`
	}
	type declaracao struct {
		InfDec infDec `xml:"InfDeclaracaoPrestacaoServico"`
	}
	type valoresNfse struct {
		BaseCalculo string `xml:"BaseCalculo"`
	}
	type infNfse struct {
		Numero            string      `xml:"Numero"`
		CodigoVerificacao string      `xml:"CodigoVerificacao"`
		DataEmissao       string      `xml:"DataEmissao"`
		Url               string      `xml:"Url"`
		URL               string      `xml:"URL"`
		UrlNfse           string      `xml:"UrlNfse"`
		UrlVisualizacao   string      `xml:"UrlVisualizacao"`
		UrlDownload       string      `xml:"UrlDownload"`
		Link              string      `xml:"Link"`
		ValoresNfse       valoresNfse `xml:"ValoresNfse"`
		Declaracao        declaracao  `xml:"DeclaracaoPrestacaoServico"`
	}
	type nfse struct {
		InfNfse infNfse `xml:"InfNfse"`
	}
	type comp struct {
		Nfse nfse `xml:"Nfse"`
	}

	var out []render.QueriedNFSe
	dec := xml.NewDecoder(bytes.NewReader(body))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "CompNfse" {
			continue
		}
		var c comp
		if err := dec.DecodeElement(&c, &se); err != nil {
			continue
		}
		inf := c.Nfse.InfNfse
		valor := strings.TrimSpace(inf.Declaracao.InfDec.Servico.Valores.ValorServicos)
		if valor == "" {
			valor = strings.TrimSpace(inf.ValoresNfse.BaseCalculo)
		}
		nfse := render.QueriedNFSe{
			Numero:             strings.TrimSpace(inf.Numero),
			CodigoVerificacao:  strings.TrimSpace(inf.CodigoVerificacao),
			DataEmissao:        formatEmissao(strings.TrimSpace(inf.DataEmissao)),
			ValorServicos:      valor,
			RazaoSocialTomador: strings.TrimSpace(inf.Declaracao.InfDec.TomadorServico.RazaoSocial),
			URL:                firstNonEmpty(inf.Url, inf.URL, inf.UrlNfse, inf.UrlVisualizacao, inf.UrlDownload, inf.Link),
		}
		if nfse.Numero == "" && nfse.CodigoVerificacao == "" && nfse.DataEmissao == "" && nfse.ValorServicos == "" && nfse.RazaoSocialTomador == "" && nfse.URL == "" {
			continue
		}
		out = append(out, nfse)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// formatEmissao shortens an ISO 8601 timestamp ("2025-12-15T15:45:40-03:00")
// to its date portion ("2025-12-15"). Leaves any non-ISO string untouched.
func formatEmissao(s string) string {
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		return s[:10]
	}
	return s
}
