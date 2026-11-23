// Package ui renders scan progress when a human is watching: a dot
// spinner pinned to the bottom line for the target being scanned, and
// one printed line per finished target scrolling above it. Output goes
// to stderr, so stdout stays clean for JSON/SARIF. Skipped entirely when
// stderr is not a terminal, in CI, or with --no-tui.
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/0xbadshah/zephy/internal/scan"
)

// UseTUI reports whether the progress view should run. It runs for
// EVERY scan on a terminal - one target or fifty, a spinner during a
// slow image catalog is worth the same as a fleet scan - unless it is
// disabled or CI.
func UseTUI(noTUI bool) bool {
	if noTUI || os.Getenv("CI") != "" {
		return false
	}
	return StderrIsTerminal()
}

// StderrIsTerminal reports whether the progress channel is a live
// terminal - the same gate the TUI itself uses.
func StderrIsTerminal() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

type eventMsg struct {
	ev scan.ScanEvent
	ok bool
}

type model struct {
	events   <-chan scan.ScanEvent
	total    int
	done     int
	findings int
	failed   int
	current  string
	listing  string
	spinner  spinner.Model
	started  time.Time
}

var (
	dim  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	okS  = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	warn = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

func NewProgress(total int, events <-chan scan.ScanEvent) *tea.Program {
	m := model{
		events:  events,
		total:   total,
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
		started: time.Now(),
	}
	return tea.NewProgram(m, tea.WithOutput(os.Stderr))
}

func (m model) Init() tea.Cmd {
	// the wordmark leads the scrollback: identity on screen within the
	// first frame, while the org listing still runs under the spinner.
	// Every print is sequenced before the next event is read: commands in
	// a Batch run concurrently, so a fast scan's quit could overtake them
	// and drop lines.
	return tea.Batch(tea.Sequence(tea.Println(Banner()), waitEvent(m.events)), m.spinner.Tick)
}

func waitEvent(ch <-chan scan.ScanEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		return eventMsg{ev, ok}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case eventMsg:
		if !msg.ok {
			return m, tea.Quit
		}
		ev := msg.ev
		switch ev.Kind {
		case "listing":
			// the org expansion is happening under the spinner: the UI
			// is already live, the network wait is never silent
			m.listing = ev.Target
			return m, waitEvent(m.events)
		case "listed":
			m.listing = ""
			return m, tea.Sequence(
				tea.Println(fmt.Sprintf(" %s · %s", ev.Target, dim.Render(fmt.Sprintf("%d active repos", ev.Total)))),
				waitEvent(m.events))
		case "targets":
			m.total = ev.Total
			return m, waitEvent(m.events)
		case "list-fail":
			return m, tea.Quit
		case "start":
			m.current = ev.Target
			return m, waitEvent(m.events)
		case "done":
			m.done++
			m.findings += ev.Findings
			if ev.Err != "" {
				m.failed++
			}
			m.current = ""
			// the finished target prints above the spinner line and
			// scrolls up as permanent, greppable history
			return m, tea.Sequence(tea.Println(statusLine(ev)), waitEvent(m.events))
		case "final":
			summary := fmt.Sprintf("%d/%d scanned · %d findings", m.done, m.total, m.findings)
			if m.failed > 0 {
				summary += fmt.Sprintf(" · %d failed", m.failed)
			}
			summary += fmt.Sprintf(" · %s", time.Since(m.started).Round(time.Second))
			// Sequence, not Batch: the summary must print before quit
			return m, tea.Sequence(tea.Println(okS.Render(" ✔ ")+summary), tea.Quit)
		}
		return m, waitEvent(m.events)
	}
	return m, nil
}

// statusLine renders one finished target for the scrolling log.
func statusLine(ev scan.ScanEvent) string {
	switch {
	case ev.Err != "":
		return warn.Render(" ✗ "+display(ev.Target)) + dim.Render(" · "+ev.Err)
	case ev.Findings > 0:
		n := fmt.Sprintf("%d findings", ev.Findings)
		if ev.Findings == 1 {
			n = "1 finding"
		}
		return " " + okS.Render("✓") + " " + display(ev.Target) + dim.Render(" · "+n)
	default:
		return dim.Render(" · " + display(ev.Target))
	}
}

// display strips the internal scheme plumbing from a target: forge and
// github labels also lose the redundant github.com host.
func display(target string) string {
	scheme, rest, ok := strings.Cut(target, ":")
	if !ok {
		return target
	}
	switch scheme {
	case "forge", "github":
		return strings.TrimPrefix(rest, "github.com/")
	case "repo":
		// "." and ".." say nothing: name the folder instead
		if strings.Trim(rest, "./") == "" {
			if abs, err := filepath.Abs(rest); err == nil {
				return filepath.Base(abs)
			}
		}
		return rest
	default:
		return rest
	}
}

// View is just the live line: spinner and whatever state the scan is in.
// The trail of finished targets lives in the terminal's scrollback,
// printed by tea.Println — nothing is padded, nothing is hidden.
func (m model) View() tea.View {
	var s string
	switch {
	case m.listing != "":
		s = fmt.Sprintf("%s listing %s…", m.spinner.View(), m.listing)
	case m.total == 0:
		s = fmt.Sprintf("%s starting…", m.spinner.View())
	case m.done >= m.total && m.current == "":
		s = ""
	default:
		cur := display(m.current)
		if cur == "" {
			cur = "starting…"
		}
		counter := fmt.Sprintf("%d/%d · %d findings", m.done, m.total, m.findings)
		if m.failed > 0 {
			counter += fmt.Sprintf(" · %d failed", m.failed)
		}
		s = fmt.Sprintf("%s %s %s", m.spinner.View(), cur, dim.Render(counter))
	}
	return tea.NewView(s)
}

// RunProgress drives the program until the scan's final event arrives.
// Call it in a goroutine and wait on the returned channel.
func RunProgress(p *tea.Program) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		_, _ = p.Run()
		close(done)
	}()
	return done
}
