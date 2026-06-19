// Package branchpicker is a Bubble Tea TUI for selecting local branches to delete.
package branchpicker

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/McBrideMusings/wtree/internal/gitwt"
)

type viewMode int

const (
	modeList viewMode = iota
	modeConfirm
)

var (
	styleSelected     = lipgloss.NewStyle().Reverse(true)
	styleFooter       = lipgloss.NewStyle().Faint(true)
	styleFooterKey    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleMerged       = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	styleStale        = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleNotYours     = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	styleName         = lipgloss.NewStyle().Bold(true)
	styleAge          = lipgloss.NewStyle().Faint(true)
	styleCheckOn      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleCheckOff     = lipgloss.NewStyle().Faint(true)
	styleConfirmTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1"))
	styleArrow        = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	styleParens       = lipgloss.NewStyle().Faint(true)
	styleCount        = lipgloss.NewStyle().Faint(true)
)

type model struct {
	branches  []gitwt.Branch
	selected  []bool
	cursor    int
	width     int
	mode      viewMode
	confirmed bool
	finished  bool
}

func newModel(branches []gitwt.Branch) model {
	selected := make([]bool, len(branches))
	for i := range selected {
		selected[i] = true
	}
	return model{branches: branches, selected: selected}
}

// Run shows the branch cleanup picker and returns the branches the user confirmed
// for deletion. Returns nil if the user cancelled with no selection.
func Run(ctx context.Context, branches []gitwt.Branch) ([]gitwt.Branch, error) {
	if len(branches) == 0 {
		return nil, nil
	}
	prog := tea.NewProgram(newModel(branches), tea.WithContext(ctx), tea.WithOutput(os.Stderr))
	final, err := prog.Run()
	if err != nil {
		return nil, err
	}
	fm := final.(model)
	if !fm.confirmed {
		return nil, nil
	}
	var result []gitwt.Branch
	for i, sel := range fm.selected {
		if sel {
			result = append(result, fm.branches[i])
		}
	}
	return result, nil
}

func branchTag(br gitwt.Branch) string {
	switch {
	case br.IsMerged:
		return styleMerged.Render("merged")
	case br.IsStale:
		return styleStale.Render("stale")
	default:
		return styleNotYours.Render("not yours")
	}
}

// renderBranchRow renders "<prefix><name>  (tag · age)" for a branch, omitting
// the age suffix if it would overflow width.
func renderBranchRow(prefix string, br gitwt.Branch, width int) string {
	row := prefix + styleName.Render(br.Name) + "  " + styleParens.Render("(") + branchTag(br)
	closer := styleParens.Render(")")
	if br.AgeStr != "" {
		extra := styleAge.Render(" · " + br.AgeStr)
		if lipgloss.Width(row+extra+closer) <= width {
			row += extra
		}
	}
	return row + closer
}

func footerKey(key, desc string) string {
	return styleFooterKey.Render(key) + styleFooter.Render(": "+desc)
}

func renderFooter(items ...string) string {
	return "  " + strings.Join(items, styleFooter.Render(" · "))
}

// countSelected returns how many entries in m.selected are true.
func (m model) countSelected() int {
	n := 0
	for _, s := range m.selected {
		if s {
			n++
		}
	}
	return n
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		switch m.mode {
		case modeConfirm:
			switch msg.String() {
			case "y", "enter":
				m.confirmed = true
				m.finished = true
				return m, tea.Quit
			case "n", "esc", "q", "ctrl+c":
				m.mode = modeList
			}
		default:
			switch msg.String() {
			case "ctrl+c", "q", "esc":
				m.finished = true
				return m, tea.Quit
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				if m.cursor < len(m.branches)-1 {
					m.cursor++
				}
			case " ":
				m.selected[m.cursor] = !m.selected[m.cursor]
			case "a":
				anySelected := m.countSelected() > 0
				for i := range m.selected {
					m.selected[i] = !anySelected
				}
			case "enter":
				if m.countSelected() > 0 {
					m.mode = modeConfirm
				}
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
	if m.mode == modeConfirm {
		return m.viewConfirm(width)
	}
	return m.viewList(width)
}

func (m model) viewList(width int) string {
	var b strings.Builder

	count := m.countSelected()
	b.WriteString(fmt.Sprintf("Clean up local branches  %s\n",
		styleCount.Render(fmt.Sprintf("(%d/%d selected)", count, len(m.branches)))))

	for i, br := range m.branches {
		var checkbox string
		if m.selected[i] {
			checkbox = styleCheckOn.Render("[✓]")
		} else {
			checkbox = styleCheckOff.Render("[ ]")
		}

		var prefix string
		if i == m.cursor {
			prefix = "  " + styleArrow.Render("▸") + " " + checkbox + " "
		} else {
			prefix = "    " + checkbox + " "
		}

		row := renderBranchRow(prefix, br, width)

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

	b.WriteString(renderFooter(
		footerKey("↑/↓ j/k", "navigate"),
		footerKey("space", "toggle"),
		footerKey("a", "toggle all"),
		footerKey("enter", "confirm"),
		footerKey("q/esc", "quit"),
	))
	return b.String()
}

func (m model) viewConfirm(width int) string {
	var b strings.Builder

	var toDelete []gitwt.Branch
	for i, s := range m.selected {
		if s {
			toDelete = append(toDelete, m.branches[i])
		}
	}

	n := len(toDelete)
	noun := "branch"
	if n != 1 {
		noun = "branches"
	}
	b.WriteString(styleConfirmTitle.Render(fmt.Sprintf("  Delete %d local %s?", n, noun)))
	b.WriteString("\n\n")

	for _, br := range toDelete {
		b.WriteString(renderBranchRow("    ", br, width))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(renderFooter(
		footerKey("y/enter", "delete"),
		footerKey("n/esc", "back"),
	))
	return b.String()
}
