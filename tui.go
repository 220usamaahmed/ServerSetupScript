package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	sTitle  = lipgloss.NewStyle().Bold(true)
	sDim    = lipgloss.NewStyle().Faint(true)
	sOK     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	sWarn   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	sBad    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	sRun    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	sCursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5"))
)

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type phase int

const (
	phaseSelect phase = iota
	phaseRun
	phaseDone
)

type tickMsg time.Time
type eventMsg Event

type model struct {
	host    string
	info    Info
	steps   []*Step
	visible []*Step
	sel     map[string]bool
	save    func(map[string]bool)
	runner  *Runner
	logDir  string

	cursor int
	note   string
	phase  phase
	items  []*Item
	byID   map[string]*Item
	events chan Event
	cancel context.CancelFunc
	start  time.Time
	spin   int

	Cancelled bool
}

func newModel(host string, info Info, steps []*Step, sel map[string]bool, runner *Runner, save func(map[string]bool)) *model {
	m := &model{host: host, info: info, steps: steps, sel: sel, runner: runner, save: save, logDir: runner.LogDir}
	for _, s := range steps {
		if !s.Hidden {
			m.visible = append(m.visible, s)
		}
	}
	return m
}

func (m *model) Init() tea.Cmd { return nil }

func tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *model) waitEvent() tea.Cmd {
	ch := m.events
	return func() tea.Msg { return eventMsg(<-ch) }
}

// startRun launches the runner for every Pending item.
func (m *model) startRun() tea.Cmd {
	var todo []*Step
	prior := map[string]Status{}
	for _, it := range m.items {
		if it.Status == Pending {
			todo = append(todo, it.Step)
		} else {
			prior[it.Step.ID] = it.Status
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.events = make(chan Event, 256)
	ch := m.events
	go m.runner.Run(ctx, todo, prior, func(e Event) { ch <- e })
	m.phase = phaseRun
	m.start = time.Now()
	return tea.Batch(m.waitEvent(), tick())
}

func (m *model) buildItems() error {
	plan, err := Plan(m.steps, m.sel)
	if err != nil {
		return err
	}
	m.items, m.byID = nil, map[string]*Item{}
	for _, s := range plan {
		it := &Item{Step: s}
		m.items = append(m.items, it)
		m.byID[s.ID] = it
	}
	return nil
}

func (m *model) anyFailed() bool {
	for _, it := range m.items {
		if it.Status == Failed {
			return true
		}
	}
	return false
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		if m.phase == phaseRun {
			m.spin++
			return m, tick()
		}
	case eventMsg:
		ev := Event(msg)
		if ev.Kind == EvAllDone {
			m.phase = phaseDone
			return m, nil
		}
		it := m.byID[ev.ID]
		switch ev.Kind {
		case EvStart:
			it.Status, it.Started = Running, time.Now()
		case EvLine:
			it.Last = ev.Line
		case EvFinish:
			it.Status, it.Detail, it.Tail, it.Log = ev.Status, ev.Detail, ev.Tail, ev.Log
			if !it.Started.IsZero() {
				it.Elapsed = time.Since(it.Started)
			}
		}
		return m, m.waitEvent()
	case tea.KeyMsg:
		k := msg.String()
		if k == "ctrl+c" {
			if m.cancel != nil {
				m.cancel()
			}
			m.Cancelled = true
			return m, tea.Quit
		}
		switch m.phase {
		case phaseSelect:
			return m.updateSelect(k)
		case phaseDone:
			switch k {
			case "q", "enter", "esc":
				return m, tea.Quit
			case "r":
				if m.anyFailed() {
					for _, it := range m.items {
						if it.Status == Failed || (it.Status == Skipped && strings.HasPrefix(it.Detail, "requires ")) {
							it.Status, it.Detail, it.Tail, it.Last = Pending, "", nil, ""
						}
					}
					return m, m.startRun()
				}
			}
		case phaseRun:
			if k == "q" {
				m.cancel()
				m.Cancelled = true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m *model) updateSelect(k string) (tea.Model, tea.Cmd) {
	m.note = ""
	switch k {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.visible)-1 {
			m.cursor++
		}
	case " ", "x":
		s := m.visible[m.cursor]
		if m.sel[s.ID] {
			m.sel[s.ID] = false
			var off []string
			for _, d := range Dependents(m.steps, s.ID) {
				if m.sel[d] {
					m.sel[d] = false
					off = append(off, d)
				}
			}
			if len(off) > 0 {
				m.note = "also unticked (they require " + s.ID + "): " + strings.Join(off, ", ")
			}
		} else {
			m.sel[s.ID] = true
			var on []string
			for _, r := range Requirements(m.steps, s.ID) {
				if !m.sel[r] {
					m.sel[r] = true
					if !stepIndex(m.steps)[r].Hidden {
						on = append(on, r)
					}
				}
			}
			if len(on) > 0 {
				m.note = "also ticked (required by " + s.ID + "): " + strings.Join(on, ", ")
			}
		}
	case "a":
		for _, s := range m.visible {
			m.sel[s.ID] = true
		}
	case "n":
		for _, s := range m.visible {
			m.sel[s.ID] = false
		}
	case "q", "esc":
		m.Cancelled = true
		return m, tea.Quit
	case "enter":
		if err := m.buildItems(); err != nil {
			m.note = err.Error()
			return m, nil
		}
		if len(m.items) == 0 {
			m.note = "nothing selected"
			return m, nil
		}
		m.save(m.sel)
		return m, m.startRun()
	}
	return m, nil
}

func (m *model) header() string {
	return sTitle.Render("sss") + "  " + m.host + "\n" + sDim.Render(m.info.String()) + "\n\n"
}

func (m *model) View() string {
	var b strings.Builder
	b.WriteString(m.header())
	switch m.phase {
	case phaseSelect:
		b.WriteString("Choose what to set up:\n\n")
		for i, s := range m.visible {
			cur := "  "
			if i == m.cursor {
				cur = sCursor.Render("▸ ")
			}
			box := "[ ]"
			if m.sel[s.ID] {
				box = sOK.Render("[x]")
			}
			line := fmt.Sprintf("%s%s %s", cur, box, s.Name)
			b.WriteString(line)
			if s.NeedsRoot && m.info.Root == "none" {
				b.WriteString(sWarn.Render("  (needs root, unavailable: will be skipped)"))
			}
			b.WriteString("\n")
			if i == m.cursor && s.Description != "" {
				b.WriteString("      " + sDim.Render(s.Description) + "\n")
			}
		}
		b.WriteString("\n")
		if m.note != "" {
			b.WriteString(sWarn.Render(m.note) + "\n\n")
		}
		b.WriteString(sDim.Render("↑/↓ move · space toggle · a all · n none · enter start · q quit") + "\n")
	default:
		done := 0
		for _, it := range m.items {
			if it.Status != Pending && it.Status != Running {
				done++
			}
		}
		if m.phase == phaseRun {
			fmt.Fprintf(&b, "Running step %d of %d · %s\n\n", min(done+1, len(m.items)), len(m.items), time.Since(m.start).Round(time.Second))
		} else {
			b.WriteString(sTitle.Render("Finished") + "\n\n")
		}
		for _, it := range m.items {
			b.WriteString(m.renderItem(it))
		}
		b.WriteString("\n")
		if m.phase == phaseDone {
			ok, skipped, failed := 0, 0, 0
			for _, it := range m.items {
				switch it.Status {
				case Done, Already:
					ok++
				case Skipped:
					skipped++
				case Failed:
					failed++
				}
			}
			summary := fmt.Sprintf("%d ok · %d skipped · %d failed", ok, skipped, failed)
			if failed > 0 {
				summary = sBad.Render(summary)
			} else {
				summary = sOK.Render(summary)
			}
			b.WriteString(summary + "\n" + sDim.Render("logs: "+m.logDir) + "\n")
			if failed > 0 {
				b.WriteString(sDim.Render("r retry failed · q quit") + "\n")
			} else {
				b.WriteString(sDim.Render("q quit") + "\n")
			}
		} else {
			b.WriteString(sDim.Render("ctrl+c to abort") + "\n")
		}
	}
	return b.String()
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func (m *model) renderItem(it *Item) string {
	var icon, detail string
	switch it.Status {
	case Pending:
		icon = sDim.Render("○")
	case Running:
		icon = sRun.Render(spinFrames[m.spin%len(spinFrames)])
		detail = sDim.Render(trunc(it.Last, 70))
	case Done:
		icon = sOK.Render("✔")
		detail = sDim.Render(trunc(it.Detail, 70))
	case Already:
		icon = sOK.Render("✔")
		detail = sDim.Render("already done: " + trunc(it.Detail, 56))
	case Skipped:
		icon = sWarn.Render("↷")
		detail = sWarn.Render("skipped: " + trunc(it.Detail, 60))
	case Failed:
		icon = sBad.Render("✘")
		detail = sBad.Render(trunc(it.Detail, 70))
	}
	line := fmt.Sprintf("  %s %-40s %s\n", icon, trunc(it.Step.Name, 40), detail)
	if it.Status == Failed {
		tail := it.Tail
		if len(tail) > 5 {
			tail = tail[len(tail)-5:]
		}
		for _, l := range tail {
			line += "      " + sDim.Render(trunc(l, 100)) + "\n"
		}
		if it.Log != "" {
			line += "      " + sDim.Render("log: "+it.Log) + "\n"
		}
	}
	return line
}

func (m *model) ExitCode() int {
	if m.Cancelled {
		return 130
	}
	if m.anyFailed() {
		return 1
	}
	return 0
}

func runTUI(m *model) (int, error) {
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		return 2, err
	}
	return m.ExitCode(), nil
}
