package render

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

type humanRenderer struct {
	w      io.Writer
	styles humanStyles
}

func (r *humanRenderer) Init(path string, created []string) error {
	r.headline("projeto inicializado em " + path)
	for _, f := range created {
		r.row("criado", f)
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
	r.section("workspace")
	r.row("ambiente", s.Ambiente)
	r.row("wsdl", s.WSDLURL)
	r.section("prestador")
	r.row("razão social", s.RazaoSocial)
	r.row("CNPJ", s.CNPJ)
	r.row("inscrição municipal", s.InscricaoMunicipal)
	r.section("serviço")
	r.row("série RPS", s.SerieRPS)
	r.row("próximo nº RPS", strconv.Itoa(s.ProximoNumeroRPS))
	r.row("alíquota ISS", strconv.FormatFloat(s.AliquotaISS, 'f', 2, 64)+"%")
	r.row("código município", s.CodigoMunicipio)
	auth := "—"
	switch {
	case s.UsingCertificate && s.UsingBasicAuth:
		auth = "certificado A1 + usuário/senha"
	case s.UsingCertificate:
		auth = "certificado A1"
	case s.UsingBasicAuth:
		auth = "usuário/senha"
	}
	r.section("autenticação")
	r.row("método", auth)
	return nil
}

func (r *humanRenderer) Emit(info EmitInfo) error {
	if info.DryRun {
		r.note("dry-run — sem chamada SOAP, sem incremento do contador")
		r.row("RPS número", strconv.Itoa(info.NumeroRPS))
		r.emitVerbose(info)
		return nil
	}
	if !info.Sucesso {
		r.headlineErr("falha na emissão")
		r.row("RPS número", strconv.Itoa(info.NumeroRPS))
		printMensagens(r.w, info.Mensagens)
		r.emitVerbose(info)
		return nil
	}
	if info.Pendente {
		r.headline("solicitação de NFS-e enviada")
	} else {
		r.headline("NFS-e emitida")
	}
	r.row("RPS número", strconv.Itoa(info.NumeroRPS))
	if info.NumeroNFSe != "" && (info.NFSe == nil || info.NFSe.Numero == "") {
		r.row("NFS-e número", info.NumeroNFSe)
	}
	if info.NFSe != nil {
		r.printNFSe(*info.NFSe)
	}
	if info.Pendente {
		r.note("solicitação recebida; confirmação da NFS-e ainda pendente")
	}
	printMensagens(r.w, info.Mensagens)
	r.emitVerbose(info)
	return nil
}

func (r *humanRenderer) emitVerbose(info EmitInfo) {
	if !info.Verbose {
		return
	}
	printBlock(r.w, "XML assinado", info.SignedXML)
	printBlock(r.w, "resposta SOAP", info.Response)
	printBlock(r.w, "resposta consulta RPS", info.ConfirmXML)
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
	r.table(
		[]string{"Número", "Cód. Verif.", "Emissão", "Valor", "Tomador"},
		queryRows(info.NFSes),
	)
	return nil
}

func (r *humanRenderer) Cancel(info CancelInfo) error {
	if !info.Sucesso {
		r.headlineErr("falha no cancelamento")
		r.row("NFS-e número", info.NumeroNFSe)
		printMensagens(r.w, info.Mensagens)
		return nil
	}
	r.headline("NFS-e cancelada")
	r.row("NFS-e número", info.NumeroNFSe)
	r.row("código", strconv.Itoa(info.Codigo))
	return nil
}

func (r *humanRenderer) headline(s string) {
	fmt.Fprintln(r.w, r.styles.success.Render("OK:"), r.styles.header.Render(s))
}

func (r *humanRenderer) headlineErr(s string) {
	fmt.Fprintln(r.w, r.styles.error.Render("ERRO:"), r.styles.header.Render(s))
}

func (r *humanRenderer) note(s string) {
	fmt.Fprintln(r.w, r.styles.warning.Render("dica:"), s)
}

func (r *humanRenderer) section(s string) {
	fmt.Fprintf(r.w, "%s %s\n", r.styles.rail.Render("│"), r.styles.muted.Render(s))
}

func (r *humanRenderer) row(key, value string) {
	label := r.styles.accent.Render(padRight(key, 22))
	fmt.Fprintf(r.w, "%s %s %s\n", r.styles.rail.Render("│"), label, value)
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

func printBlock(w io.Writer, title, body string) {
	if body == "" {
		return
	}
	fmt.Fprintf(w, "%s:\n%s\n", title, body)
}

func (r *humanRenderer) printNFSe(n QueriedNFSe) {
	if n.Numero != "" {
		r.row("NFS-e número", n.Numero)
	}
	if n.CodigoVerificacao != "" {
		r.row("cód. verificação", n.CodigoVerificacao)
	}
	if n.DataEmissao != "" {
		r.row("emissão", n.DataEmissao)
	}
	if n.ValorServicos != "" {
		r.row("valor", n.ValorServicos)
	}
	if n.URL != "" {
		r.row("URL", n.URL)
	}
}

func (r *humanRenderer) table(headers []string, rows [][]string) {
	widths := tableWidths(headers, rows)
	fmt.Fprint(r.w, r.styles.rail.Render("│ "))
	for i, h := range headers {
		if i > 0 {
			fmt.Fprint(r.w, "  ")
		}
		fmt.Fprint(r.w, r.styles.header.Render(padRight(h, widths[i])))
	}
	fmt.Fprintln(r.w)
	fmt.Fprint(r.w, r.styles.rail.Render("│ "))
	for i, width := range widths {
		if i > 0 {
			fmt.Fprint(r.w, "  ")
		}
		fmt.Fprint(r.w, r.styles.rail.Render(strings.Repeat("─", width)))
	}
	fmt.Fprintln(r.w)
	for _, row := range rows {
		fmt.Fprint(r.w, r.styles.rail.Render("│ "))
		for i, cell := range row {
			if i > 0 {
				fmt.Fprint(r.w, "  ")
			}
			fmt.Fprint(r.w, padRight(cell, widths[i]))
		}
		fmt.Fprintln(r.w)
	}
}

func queryRows(nfses []QueriedNFSe) [][]string {
	rows := make([][]string, 0, len(nfses))
	for _, n := range nfses {
		rows = append(rows, []string{n.Numero, n.CodigoVerificacao, n.DataEmissao, n.ValorServicos, n.RazaoSocialTomador})
	}
	return rows
}

func tableWidths(headers []string, rows [][]string) []int {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = lipgloss.Width(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				continue
			}
			if w := lipgloss.Width(cell); w > widths[i] {
				widths[i] = w
			}
		}
	}
	return widths
}

func padRight(s string, width int) string {
	if lipgloss.Width(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-lipgloss.Width(s))
}

type humanStyles struct {
	header  lipgloss.Style
	muted   lipgloss.Style
	rail    lipgloss.Style
	accent  lipgloss.Style
	success lipgloss.Style
	warning lipgloss.Style
	error   lipgloss.Style
}

func newHumanStyles(color bool) humanStyles {
	if !color {
		return humanStyles{
			header:  lipgloss.NewStyle(),
			muted:   lipgloss.NewStyle(),
			rail:    lipgloss.NewStyle(),
			accent:  lipgloss.NewStyle(),
			success: lipgloss.NewStyle(),
			warning: lipgloss.NewStyle(),
			error:   lipgloss.NewStyle(),
		}
	}
	return humanStyles{
		header:  lipgloss.NewStyle().Bold(true),
		muted:   lipgloss.NewStyle().Foreground(lipgloss.Color("#8A8F98")),
		rail:    lipgloss.NewStyle().Foreground(lipgloss.Color("#3A3F46")),
		accent:  lipgloss.NewStyle().Foreground(lipgloss.Color("#8AB4D6")),
		success: lipgloss.NewStyle().Foreground(lipgloss.Color("#8CBF88")).Bold(true),
		warning: lipgloss.NewStyle().Foreground(lipgloss.Color("#D6B97A")),
		error:   lipgloss.NewStyle().Foreground(lipgloss.Color("#D7827E")).Bold(true),
	}
}

func colorEnabled(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}
