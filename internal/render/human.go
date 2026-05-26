package render

import (
	"fmt"
	"io"
	"strconv"

	"github.com/fatih/color"
	"github.com/jedib0t/go-pretty/v6/table"
)

type humanRenderer struct {
	w io.Writer
}

func (r *humanRenderer) Init(path string, created []string) error {
	r.headline("projeto inicializado em " + path)
	for _, f := range created {
		fmt.Fprintln(r.w, "  -", f)
	}
	fmt.Fprintln(r.w, "dica: edite config.toml com os dados da sua empresa antes de usar.")
	return nil
}

func (r *humanRenderer) Env(name string) error {
	r.headline("ambiente alterado para " + name)
	return nil
}

func (r *humanRenderer) Status(s StatusInfo) error {
	r.headline("status do nfe")
	kv(r.w, "ambiente", s.Ambiente)
	kv(r.w, "wsdl", s.WSDLURL)
	kv(r.w, "razão social", s.RazaoSocial)
	kv(r.w, "CNPJ", s.CNPJ)
	kv(r.w, "inscrição municipal", s.InscricaoMunicipal)
	kv(r.w, "série RPS", s.SerieRPS)
	kv(r.w, "próximo nº RPS", strconv.Itoa(s.ProximoNumeroRPS))
	kv(r.w, "alíquota ISS", strconv.FormatFloat(s.AliquotaISS, 'f', 2, 64)+"%")
	kv(r.w, "código município", s.CodigoMunicipio)
	auth := "—"
	switch {
	case s.UsingCertificate && s.UsingBasicAuth:
		auth = "certificado A1 + usuário/senha"
	case s.UsingCertificate:
		auth = "certificado A1"
	case s.UsingBasicAuth:
		auth = "usuário/senha"
	}
	kv(r.w, "autenticação", auth)
	return nil
}

func (r *humanRenderer) Emit(info EmitInfo) error {
	if info.DryRun {
		r.note("dry-run — sem chamada SOAP, sem incremento do contador")
		kv(r.w, "RPS número", strconv.Itoa(info.NumeroRPS))
		return nil
	}
	if !info.Sucesso {
		r.headlineErr("falha na emissão")
		kv(r.w, "RPS número", strconv.Itoa(info.NumeroRPS))
		printMensagens(r.w, info.Mensagens)
		return nil
	}
	r.headline("NFS-e emitida")
	kv(r.w, "RPS número", strconv.Itoa(info.NumeroRPS))
	if info.NumeroNFSe != "" {
		kv(r.w, "NFS-e número", info.NumeroNFSe)
	}
	printMensagens(r.w, info.Mensagens)
	return nil
}

func (r *humanRenderer) Query(info QueryInfo) error {
	if !info.Sucesso {
		r.headlineErr("falha na consulta")
		printMensagens(r.w, info.Mensagens)
		return nil
	}
	if len(info.NFSes) == 0 {
		r.note("nenhuma NFS-e encontrada")
		return nil
	}
	tbl := table.NewWriter()
	tbl.SetOutputMirror(r.w)
	tbl.AppendHeader(table.Row{"Número", "Cód. Verif.", "Emissão", "Valor", "Tomador"})
	for _, n := range info.NFSes {
		tbl.AppendRow(table.Row{n.Numero, n.CodigoVerificacao, n.DataEmissao, n.ValorServicos, n.RazaoSocialTomador})
	}
	tbl.SetStyle(table.StyleLight)
	tbl.Render()
	return nil
}

func (r *humanRenderer) Cancel(info CancelInfo) error {
	if !info.Sucesso {
		r.headlineErr("falha no cancelamento")
		kv(r.w, "NFS-e número", info.NumeroNFSe)
		printMensagens(r.w, info.Mensagens)
		return nil
	}
	r.headline("NFS-e cancelada")
	kv(r.w, "NFS-e número", info.NumeroNFSe)
	kv(r.w, "código", strconv.Itoa(info.Codigo))
	return nil
}

func (r *humanRenderer) headline(s string) {
	c := color.New(color.FgGreen, color.Bold)
	fmt.Fprintln(r.w, c.Sprint("OK:"), s)
}

func (r *humanRenderer) headlineErr(s string) {
	c := color.New(color.FgRed, color.Bold)
	fmt.Fprintln(r.w, c.Sprint("ERRO:"), s)
}

func (r *humanRenderer) note(s string) {
	c := color.New(color.FgYellow)
	fmt.Fprintln(r.w, c.Sprint("dica:"), s)
}

func printMensagens(w io.Writer, ms []MensagemRetorno) {
	if len(ms) == 0 {
		return
	}
	fmt.Fprintln(w, "mensagens:")
	for _, m := range ms {
		fmt.Fprintf(w, "  - %s: %s\n", m.Codigo, m.Mensagem)
		if m.Correcao != "" {
			fmt.Fprintf(w, "    correção: %s\n", m.Correcao)
		}
	}
}

func kv(w io.Writer, key, value string) {
	label := color.New(color.FgCyan).Sprintf("%-22s", key)
	fmt.Fprintf(w, "  %s %s\n", label, value)
}
