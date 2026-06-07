package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/caian-org/nfe/internal/service"
)

type emitProgress struct {
	w           io.Writer
	updates     chan emitProgressUpdate
	done        chan error
	interactive bool
	disabled    bool
}

type emitProgressUpdate struct {
	step   string
	status service.ProgressStatus
	detail string
}

func newEmitProgress(cmd *cobra.Command, gf *globalFlags, notaName string, verbose bool) *emitProgress {
	if gf.json {
		return &emitProgress{disabled: true}
	}

	stderr := cmd.ErrOrStderr()
	stdout := cmd.OutOrStdout()
	interactive := !verbose && isTerminalWriter(stdout) && isTerminalWriter(stderr)
	p := &emitProgress{
		w:           stderr,
		interactive: interactive,
		updates:     make(chan emitProgressUpdate, 32),
		done:        make(chan error, 1),
	}

	if interactive {
		model := newEmitProgressModel("nfe emit · "+notaName, p.updates)
		go func() {
			_, err := tea.NewProgram(model, tea.WithOutput(stderr), tea.WithContext(cmd.Context())).Run()
			p.done <- err
		}()
		return p
	}

	fmt.Fprintf(stderr, "nfe emit · %s\n", notaName)
	return p
}

func (p *emitProgress) ServiceProgress() service.ProgressFunc {
	if p.disabled {
		return nil
	}
	return func(event service.ProgressEvent) {
		p.Report(event.Step, event.Status, event.Detail)
	}
}

func (p *emitProgress) Report(step string, status service.ProgressStatus, detail string) {
	if p.disabled {
		return
	}
	u := emitProgressUpdate{step: step, status: status, detail: detail}
	if p.interactive {
		select {
		case p.updates <- u:
		default:
		}
		return
	}
	fmt.Fprintf(p.w, "%s %-12s %s\n", plainProgressGlyph(status), step, detail)
}

func (p *emitProgress) Close() {
	if p.disabled {
		return
	}
	if p.interactive {
		close(p.updates)
		select {
		case <-p.done:
		case <-time.After(2 * time.Second):
		}
		return
	}
	fmt.Fprintln(p.w)
}

func plainProgressGlyph(status service.ProgressStatus) string {
	switch status {
	case service.ProgressDone:
		return "✓"
	case service.ProgressSkipped:
		return "·"
	case service.ProgressFailed:
		return "!"
	default:
		return "→"
	}
}

func isTerminalWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

type emitProgressModel struct {
	title   string
	order   []string
	labels  map[string]string
	steps   map[string]emitProgressStep
	spin    spinner.Model
	updates <-chan emitProgressUpdate
}

type emitProgressStep struct {
	status  service.ProgressStatus
	detail  string
	start   time.Time
	elapsed time.Duration
}

type emitProgressClosed struct{}

func newEmitProgressModel(title string, updates <-chan emitProgressUpdate) emitProgressModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return emitProgressModel{
		title:   title,
		order:   []string{"nota", "xml", "assinatura", "prefeitura", "confirmação", "config"},
		labels:  map[string]string{"nota": "nota", "xml": "xml", "assinatura": "assinatura", "prefeitura": "prefeitura", "confirmação": "confirmação", "config": "config"},
		steps:   map[string]emitProgressStep{},
		spin:    sp,
		updates: updates,
	}
}

func (m emitProgressModel) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, recvEmitProgress(m.updates))
}

func recvEmitProgress(ch <-chan emitProgressUpdate) tea.Cmd {
	return func() tea.Msg {
		u, ok := <-ch
		if !ok {
			return emitProgressClosed{}
		}
		return u
	}
}

func (m emitProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(v)
		return m, cmd
	case emitProgressUpdate:
		step := m.steps[v.step]
		if step.status != service.ProgressStarted && v.status == service.ProgressStarted {
			step.start = time.Now()
		}
		if v.status == service.ProgressDone || v.status == service.ProgressSkipped || v.status == service.ProgressFailed {
			if !step.start.IsZero() {
				step.elapsed = time.Since(step.start)
			}
		}
		step.status = v.status
		step.detail = v.detail
		m.steps[v.step] = step
		return m, recvEmitProgress(m.updates)
	case emitProgressClosed:
		return m, tea.Quit
	}
	return m, nil
}

func (m emitProgressModel) View() string {
	var b strings.Builder
	b.WriteString(styleHeader.Render(m.title))
	b.WriteString("\n")
	b.WriteString(styleRail.Render(strings.Repeat("─", 56)))
	b.WriteString("\n")

	for _, step := range m.order {
		state := m.steps[step]
		label := m.labels[step]
		if label == "" {
			label = step
		}
		fmt.Fprintf(&b, "%s %-12s %s", m.glyph(state.status), label, progressDetail(state))
		if state.elapsed > 0 {
			fmt.Fprintf(&b, " %s", styleMuted.Render(humanDuration(state.elapsed)))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m emitProgressModel) glyph(status service.ProgressStatus) string {
	switch status {
	case service.ProgressDone:
		return styleSuccess.Render("✓")
	case service.ProgressSkipped:
		return styleMuted.Render("·")
	case service.ProgressFailed:
		return styleError.Render("!")
	case service.ProgressStarted:
		return styleAccent.Render(m.spin.View())
	default:
		return styleMuted.Render("·")
	}
}

func progressDetail(step emitProgressStep) string {
	switch step.status {
	case service.ProgressStarted:
		if step.detail == "" {
			return styleAccent.Render("em andamento")
		}
		return step.detail
	case service.ProgressDone:
		if step.detail == "" {
			return styleSuccess.Render("concluído")
		}
		return step.detail
	case service.ProgressSkipped:
		if step.detail == "" {
			return styleMuted.Render("ignorado")
		}
		return styleMuted.Render(step.detail)
	case service.ProgressFailed:
		if step.detail == "" {
			return styleError.Render("falhou")
		}
		return styleError.Render(step.detail)
	default:
		return styleMuted.Render("pendente")
	}
}

func humanDuration(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		min := int(d.Minutes())
		sec := int(d.Seconds()) - min*60
		return fmt.Sprintf("%dm%02ds", min, sec)
	}
	hour := int(d.Hours())
	min := int(d.Minutes()) - hour*60
	return fmt.Sprintf("%dh%02dm", hour, min)
}

var (
	styleHeader  = lipgloss.NewStyle().Bold(true)
	styleMuted   = lipgloss.NewStyle().Foreground(lipgloss.Color("#8A8F98"))
	styleRail    = lipgloss.NewStyle().Foreground(lipgloss.Color("#3A3F46"))
	styleAccent  = lipgloss.NewStyle().Foreground(lipgloss.Color("#8AB4D6"))
	styleSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("#8CBF88"))
	styleError   = lipgloss.NewStyle().Foreground(lipgloss.Color("#D7827E"))
)
