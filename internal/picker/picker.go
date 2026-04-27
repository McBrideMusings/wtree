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

	"github.com/McBrideMusings/wtree/internal/gitwt"
)

type Action int

const (
	ActionNone Action = iota
	ActionEnter
	ActionRemove
)

type Selection struct {
	Action   Action
	Worktree gitwt.Worktree
}

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
	if fm.action == ActionNone {
		return Selection{}, nil
	}
	return Selection{Action: fm.action, Worktree: fm.list[fm.cursor]}, nil
}

type statusMsg struct {
	index int
	dirty bool
	age   string
}

type model struct {
	ctx         context.Context
	prompt      string
	defAction   DefaultAction
	list        []gitwt.Worktree
	currentPath string
	cursor      int
	width       int
	status      []rowStatus
	action      Action
	finished    bool
}

type rowStatus struct {
	loaded bool
	dirty  bool
	age    string
}

var (
	styleSelected = lipgloss.NewStyle().Reverse(true)
	styleFooter   = lipgloss.NewStyle().Faint(true)
	styleDirty    = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	styleCurrent  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	styleAge      = lipgloss.NewStyle().Faint(true)
	styleBranch   = lipgloss.NewStyle().Foreground(lipgloss.Color("6")) // cyan
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
	cmds := make([]tea.Cmd, 0, len(m.list))
	for i, w := range m.list {
		cmds = append(cmds, fetchStatus(m.ctx, i, w.Path))
	}
	return tea.Batch(cmds...)
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
			m.status[msg.index] = rowStatus{loaded: true, dirty: msg.dirty, age: msg.age}
		}
		return m, nil
	case tea.KeyMsg:
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
			m.action = ActionEnter
			if m.defAction == DefaultRemove {
				m.action = ActionRemove
			}
			m.finished = true
			return m, tea.Quit
		case "x":
			m.action = ActionRemove
			m.finished = true
			return m, tea.Quit
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
		prefix := "    "
		if i == m.cursor {
			prefix = "  ▸ "
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
		core := fmt.Sprintf("%s  (%s)", name, styleBranch.Render(branchLabel))

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

	footer := fmt.Sprintf("  ↑/↓ or j/k navigate · enter: %s · x: remove · q/esc: quit", m.defAction)
	b.WriteString(styleFooter.Render(footer))
	return b.String()
}
