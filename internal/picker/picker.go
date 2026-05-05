// Package picker is the Bubble Tea TUI worktree picker.
package picker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/McBrideMusings/wtree/internal/gh"
	"github.com/McBrideMusings/wtree/internal/gitwt"
)

type Action int

const (
	ActionNone Action = iota
	ActionEnter
	ActionRemove
	ActionEditConfig
	ActionEditGlobalConfig
	ActionRemoveMerged
)

type Selection struct {
	Action    Action
	Worktree  gitwt.Worktree
	Worktrees []gitwt.Worktree // populated for ActionRemoveMerged
}

type viewMode int

const (
	modePicker viewMode = iota
	modeConfirmRemoveMerged
)

type DefaultAction string

const (
	DefaultEnter  DefaultAction = "cd"
	DefaultRemove DefaultAction = "remove"
)

// Run shows the picker for the worktrees passed in (callers filter the main
// worktree out themselves) and returns what the user picked.
func Run(ctx context.Context, prompt string, defAction DefaultAction, list []gitwt.Worktree, currentPath string) (Selection, error) {
	if len(list) == 0 {
		return Selection{}, fmt.Errorf("no worktrees")
	}
	m := newModel(ctx, prompt, defAction, list, currentPath)
	prog := tea.NewProgram(m, tea.WithContext(ctx), tea.WithOutput(os.Stderr))
	final, err := prog.Run()
	if err != nil {
		return Selection{}, err
	}
	fm := final.(model)
	switch fm.action {
	case ActionNone:
		return Selection{}, nil
	case ActionRemoveMerged:
		wts := make([]gitwt.Worktree, len(fm.mergedToRemove))
		for i, idx := range fm.mergedToRemove {
			wts[i] = fm.list[idx]
		}
		return Selection{Action: ActionRemoveMerged, Worktrees: wts}, nil
	default:
		return Selection{Action: fm.action, Worktree: fm.list[fm.cursor]}, nil
	}
}

type statusMsg struct {
	index int
	dirty bool
	age   string
}

type prStatusMsg struct {
	index  int
	number int
	state  string
	found  bool
	ghErr  bool // gh binary missing or auth failure
}

type model struct {
	ctx            context.Context
	prompt         string
	defAction      DefaultAction
	list           []gitwt.Worktree
	currentPath    string
	cursor         int
	width          int
	status         []rowStatus
	action         Action
	finished       bool
	mode           viewMode
	mergedToRemove []int
	flashMsg       string
	ghUnavailable  bool
}

type rowStatus struct {
	loaded   bool
	dirty    bool
	age      string
	prLoaded bool
	prFound  bool
	prNumber int
	prState  string
}

var (
	styleSelected  = lipgloss.NewStyle().Reverse(true)
	styleFooter    = lipgloss.NewStyle().Faint(true)
	styleFooterKey = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))  // cyan keys
	styleDirty     = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))  // yellow
	styleCurrent   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))  // green
	styleAge       = lipgloss.NewStyle().Faint(true)
	styleBranch    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))  // cyan
	styleParens    = lipgloss.NewStyle().Faint(true)
	styleName      = lipgloss.NewStyle().Bold(true)
	styleArrow     = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true) // yellow bold
	styleMerged       = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))  // magenta
	styleOpenPR       = lipgloss.NewStyle().Foreground(lipgloss.Color("12")) // bright blue
	styleClosedPR     = lipgloss.NewStyle().Faint(true)
	styleConfirmTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1")) // red bold
	styleFlash        = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))            // yellow
)

func newModel(ctx context.Context, prompt string, defAction DefaultAction, list []gitwt.Worktree, currentPath string) model {
	return model{
		ctx:         ctx,
		prompt:      prompt,
		defAction:   defAction,
		list:        list,
		currentPath: currentPath,
		status:      make([]rowStatus, len(list)),
	}
}

func (m model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.list)*2)
	for i, w := range m.list {
		cmds = append(cmds, fetchStatus(m.ctx, i, w.Path))
		cmds = append(cmds, fetchPRStatus(m.ctx, i, w.Branch))
	}
	return tea.Batch(cmds...)
}

func fetchPRStatus(parent context.Context, index int, branch string) tea.Cmd {
	return func() tea.Msg {
		if branch == "" {
			return prStatusMsg{index: index}
		}
		ctx, cancel := context.WithTimeout(parent, 10*time.Second)
		defer cancel()
		info, found, err := gh.PRForBranch(ctx, branch)
		if err != nil {
			return prStatusMsg{index: index, ghErr: true}
		}
		return prStatusMsg{index: index, number: info.Number, state: info.State, found: found}
	}
}

func fetchStatus(parent context.Context, index int, path string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, time.Second)
		defer cancel()

		dirty := false
		if out, err := exec.CommandContext(ctx, "git", "-C", path, "status", "--porcelain").Output(); err == nil {
			dirty = len(strings.TrimSpace(string(out))) > 0
		}
		age := ""
		if out, err := exec.CommandContext(ctx, "git", "-C", path, "log", "-1", "--format=%cr", "HEAD").Output(); err == nil {
			age = strings.TrimSpace(string(out))
		}
		return statusMsg{index: index, dirty: dirty, age: age}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case statusMsg:
		if msg.index >= 0 && msg.index < len(m.status) {
			st := &m.status[msg.index]
			st.loaded = true
			st.dirty = msg.dirty
			st.age = msg.age
		}
		return m, nil
	case prStatusMsg:
		if msg.ghErr {
			m.ghUnavailable = true
			return m, nil
		}
		if msg.index >= 0 && msg.index < len(m.status) {
			st := &m.status[msg.index]
			st.prLoaded = true
			st.prFound = msg.found
			st.prNumber = msg.number
			st.prState = msg.state
		}
		return m, nil
	case tea.KeyMsg:
		m.flashMsg = ""
		switch m.mode {
		case modeConfirmRemoveMerged:
			switch msg.String() {
			case "ctrl+c", "q", "n", "esc":
				m.mode = modePicker
				m.mergedToRemove = nil
			case "y", "enter":
				m.action = ActionRemoveMerged
				m.finished = true
				return m, tea.Quit
			}
		default:
			switch msg.String() {
			case "ctrl+c", "q", "esc":
				m.action = ActionNone
				m.finished = true
				return m, tea.Quit
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				if m.cursor < len(m.list)-1 {
					m.cursor++
				}
			case "enter":
				if m.defAction == DefaultRemove {
					m.action = ActionRemove
				} else {
					m.action = ActionEnter
				}
				m.finished = true
				return m, tea.Quit
			case "x":
				m.action = ActionRemove
				m.finished = true
				return m, tea.Quit
			case "D":
				var indices []int
				for i, st := range m.status {
					if st.prLoaded && st.prFound && st.prState == "MERGED" {
						indices = append(indices, i)
					}
				}
				switch {
				case len(indices) > 0:
					m.mergedToRemove = indices
					m.mode = modeConfirmRemoveMerged
				case m.ghUnavailable:
					m.flashMsg = "gh unavailable – PR status could not be loaded."
				default:
					m.flashMsg = "No merged worktrees found."
				}
			case "e":
				m.action = ActionEditConfig
				m.finished = true
				return m, tea.Quit
			case "g":
				m.action = ActionEditGlobalConfig
				m.finished = true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.finished {
		return ""
	}
	width := m.width
	if width <= 0 {
		width = 80
	}

	var b strings.Builder
	b.WriteString(m.prompt)
	b.WriteString("\n")

	for i, w := range m.list {
		var prefix string
		if i == m.cursor {
			prefix = "  " + styleArrow.Render("▸") + " "
		} else {
			prefix = "    "
		}
		name := filepath.Base(w.Path)
		branchLabel := w.Branch
		if w.Detached {
			short := w.Head
			if len(short) > 7 {
				short = short[:7]
			}
			branchLabel = "detached " + short
		}
		core := styleName.Render(name) + "  " + styleParens.Render("(") + styleBranch.Render(branchLabel) + styleParens.Render(")")

		// Compose extras in priority order: current > dirty > age. Drop
		// lower-priority ones if the row would overflow.
		var extras []string
		if w.Path == m.currentPath && m.currentPath != "" {
			extras = append(extras, styleCurrent.Render(" (current)"))
		}
		st := m.status[i]
		if st.loaded && st.dirty {
			extras = append(extras, styleDirty.Render(" *"))
		}
		if st.loaded && st.age != "" {
			extras = append(extras, styleAge.Render(" · "+st.age))
		}
		if st.prLoaded && st.prFound {
			switch st.prState {
			case "MERGED":
				extras = append(extras, styleMerged.Render(" · ✓ merged"))
			case "OPEN":
				extras = append(extras, styleOpenPR.Render(fmt.Sprintf(" · #%d", st.prNumber)))
			case "CLOSED":
				extras = append(extras, styleClosedPR.Render(" · ✗ closed"))
			}
		}

		row := prefix + core
		for _, e := range extras {
			if lipgloss.Width(row+e) > width {
				break
			}
			row += e
		}

		if i == m.cursor {
			pad := width - lipgloss.Width(row)
			if pad > 0 {
				row += strings.Repeat(" ", pad)
			}
			row = styleSelected.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}

	fk := func(key, desc string) string {
		return styleFooterKey.Render(key) + styleFooter.Render(": "+desc)
	}
	sep := styleFooter.Render(" · ")

	if m.mode == modeConfirmRemoveMerged {
		return m.viewConfirm(width, fk, sep)
	}

	if m.flashMsg != "" {
		b.WriteString(styleFlash.Render("  " + m.flashMsg))
	} else {
		footer := "  " + strings.Join([]string{
			fk("↑/↓ j/k", "navigate"),
			fk("enter", string(m.defAction)),
			fk("x", "remove"),
			fk("D", "remove merged"),
			fk("e", "local config"),
			fk("g", "global config"),
			fk("q/esc", "quit"),
		}, sep)
		b.WriteString(footer)
	}
	return b.String()
}

func (m model) viewConfirm(width int, fk func(string, string) string, sep string) string {
	var b strings.Builder
	n := len(m.mergedToRemove)
	noun := "worktree"
	if n != 1 {
		noun = "worktrees"
	}
	b.WriteString(styleConfirmTitle.Render(fmt.Sprintf("  Remove %d merged %s?", n, noun)))
	b.WriteString("\n\n")

	for _, idx := range m.mergedToRemove {
		w := m.list[idx]
		st := m.status[idx]
		name := filepath.Base(w.Path)
		row := "    " + styleName.Render(name) + "  " + styleParens.Render("(") + styleBranch.Render(w.Branch) + styleParens.Render(")")
		if st.loaded && st.age != "" {
			extra := styleAge.Render(" · " + st.age)
			if lipgloss.Width(row+extra) <= width {
				row += extra
			}
		}
		extra := styleMerged.Render(" · ✓ merged")
		if lipgloss.Width(row+extra) <= width {
			row += extra
		}
		b.WriteString(row)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	footer := "  " + strings.Join([]string{
		fk("y/enter", "confirm"),
		fk("n/esc", "cancel"),
	}, sep)
	b.WriteString(footer)
	return b.String()
}
