// Package picker is the Bubble Tea TUI worktree picker.
package picker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	ActionPull
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
func Run(ctx context.Context, prompt string, defAction DefaultAction, list []gitwt.Worktree, currentPath, mainPath, defaultBranch string) (Selection, error) {
	if len(list) == 0 {
		return Selection{}, fmt.Errorf("no worktrees")
	}
	m := newModel(ctx, prompt, defAction, list, currentPath, mainPath, defaultBranch)
	prog := tea.NewProgram(m, tea.WithContext(ctx), tea.WithOutput(os.Stderr))
	final, err := prog.Run()
	if err != nil {
		return Selection{}, err
	}
	fm := final.(model)
	switch fm.action {
	case ActionNone:
		return Selection{}, nil
	case ActionPull:
		return Selection{Action: ActionPull}, nil
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
	index           int
	dirty           bool
	uncommittedCount int
	linesAdded      int
	linesRemoved    int
	age             string
}

type prStatusMsg struct {
	index  int
	number int
	state  string
	found  bool
	ghErr  bool // gh binary missing or auth failure
}

type behindMsg struct {
	count int
	err   bool
}

type model struct {
	ctx            context.Context
	prompt         string
	defAction      DefaultAction
	list           []gitwt.Worktree
	currentPath    string
	mainPath       string // primary worktree path; "" when no main row is shown
	mainIndex      int    // index of the main row in list, or -1
	defaultBranch  string // branch compared against origin for the behind check
	cursor         int
	width          int
	status         []rowStatus
	behindLoaded   bool
	behindCount    int
	action         Action
	finished       bool
	mode           viewMode
	mergedToRemove []int
	flashMsg       string
	ghUnavailable  bool
}

type rowStatus struct {
	loaded          bool
	dirty           bool
	uncommittedCount int
	linesAdded      int
	linesRemoved    int
	age             string
	prLoaded        bool
	prFound         bool
	prNumber        int
	prState         string
}

var (
	styleSelected     = lipgloss.NewStyle().Reverse(true)
	styleFooter       = lipgloss.NewStyle().Faint(true)
	styleFooterKey    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))           // cyan keys
	styleDirty        = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))           // yellow
	styleCurrent      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))           // green
	styleAge          = lipgloss.NewStyle().Faint(true)
	styleBranch       = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))           // cyan
	styleParens       = lipgloss.NewStyle().Faint(true)
	styleName         = lipgloss.NewStyle().Bold(true)
	styleArrow        = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true) // yellow bold
	styleMerged       = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))            // magenta
	styleOpenPR       = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))           // bright blue
	styleClosedPR     = lipgloss.NewStyle().Faint(true)
	styleConfirmTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1")) // red bold
	styleFlash        = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))            // yellow
	styleAdded        = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))            // green
	styleRemoved      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))            // red
)

func newModel(ctx context.Context, prompt string, defAction DefaultAction, list []gitwt.Worktree, currentPath, mainPath, defaultBranch string) model {
	mainIndex := -1
	if mainPath != "" {
		for i, w := range list {
			if w.Path == mainPath {
				mainIndex = i
				break
			}
		}
	}
	return model{
		ctx:           ctx,
		prompt:        prompt,
		defAction:     defAction,
		list:          list,
		currentPath:   currentPath,
		mainPath:      mainPath,
		mainIndex:     mainIndex,
		defaultBranch: defaultBranch,
		status:        make([]rowStatus, len(list)),
	}
}

func (m model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.list)*2+1)
	for i, w := range m.list {
		cmds = append(cmds, fetchStatus(m.ctx, i, w.Path))
		cmds = append(cmds, fetchPRStatus(m.ctx, i, w.Branch))
	}
	if m.mainIndex >= 0 && m.defaultBranch != "" {
		cmds = append(cmds, fetchBehind(m.ctx, m.mainPath, m.defaultBranch))
	}
	return tea.Batch(cmds...)
}

func fetchBehind(parent context.Context, mainPath, defaultBranch string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 10*time.Second)
		defer cancel()
		count, err := gitwt.FetchAndCountBehind(ctx, mainPath, defaultBranch)
		if err != nil {
			return behindMsg{err: true}
		}
		return behindMsg{count: count}
	}
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
		ctx, cancel := context.WithTimeout(parent, 3*time.Second)
		defer cancel()

		git := func(args ...string) string {
			out, err := exec.CommandContext(ctx, "git", append([]string{"-C", path}, args...)...).Output()
			if err != nil {
				return ""
			}
			return strings.TrimSpace(string(out))
		}

		uncommittedCount := 0
		if s := git("status", "--porcelain"); s != "" {
			uncommittedCount = strings.Count(s, "\n") + 1
		}

		linesAdded, linesRemoved := 0, 0
		if s := git("diff", "HEAD", "--numstat"); s != "" {
			for _, line := range strings.Split(s, "\n") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					a, _ := strconv.Atoi(parts[0])
					r, _ := strconv.Atoi(parts[1])
					linesAdded += a
					linesRemoved += r
				}
			}
		}

		return statusMsg{
			index:            index,
			dirty:            uncommittedCount > 0,
			uncommittedCount: uncommittedCount,
			linesAdded:       linesAdded,
			linesRemoved:     linesRemoved,
			age:              git("log", "-1", "--format=%cr", "HEAD"),
		}
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
			st.uncommittedCount = msg.uncommittedCount
			st.linesAdded = msg.linesAdded
			st.linesRemoved = msg.linesRemoved
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
	case behindMsg:
		m.behindLoaded = !msg.err
		if !msg.err {
			m.behindCount = msg.count
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
					if m.cursor == m.mainIndex {
						m.flashMsg = "Cannot remove the primary worktree."
						break
					}
					m.action = ActionRemove
				} else {
					m.action = ActionEnter
				}
				m.finished = true
				return m, tea.Quit
			case "x":
				if m.cursor == m.mainIndex {
					m.flashMsg = "Cannot remove the primary worktree."
					break
				}
				m.action = ActionRemove
				m.finished = true
				return m, tea.Quit
			case "p":
				if m.mainIndex < 0 {
					break
				}
				switch {
				case !m.behindLoaded:
					m.flashMsg = "Checking origin…"
				case m.behindCount == 0:
					m.flashMsg = fmt.Sprintf("Primary is up to date with origin/%s.", m.defaultBranch)
				default:
					m.action = ActionPull
					m.finished = true
					return m, tea.Quit
				}
			case "D":
				var indices []int
				for i, st := range m.status {
					if i == m.mainIndex {
						continue // never sweep the primary worktree into batch removal
					}
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

	fk := func(key, desc string) string {
		return styleFooterKey.Render(key) + styleFooter.Render(": "+desc)
	}
	sep := styleFooter.Render(" · ")

	if m.mode == modeConfirmRemoveMerged {
		return m.viewConfirm(width, fk, sep)
	}

	// pass 1: collect plain-text cell values and max column widths
	type cellData struct {
		name       string
		branch     string
		isCurrent  bool
		filesStr   string
		addedStr   string
		removedStr string
		ageStr     string
		prLoaded   bool
		prFound    bool
		prNumber   int
		prState    string
	}

	cells := make([]cellData, len(m.list))
	var maxNameW, maxBranchW, maxFilesW, maxAddedW, maxRemovedW, maxAgeW int

	for i, w := range m.list {
		c := &cells[i]
		c.name = filepath.Base(w.Path)
		c.isCurrent = w.Path == m.currentPath && m.currentPath != ""

		nameW := len(c.name)
		if c.isCurrent {
			nameW += len(" (current)")
		}
		maxNameW = max(maxNameW, nameW)

		if w.Detached {
			short := w.Head
			if len(short) > 7 {
				short = short[:7]
			}
			c.branch = "detached " + short
		} else {
			c.branch = w.Branch
		}
		maxBranchW = max(maxBranchW, len(c.branch))

		st := m.status[i]
		if st.loaded && st.dirty {
			noun := "files"
			if st.uncommittedCount == 1 {
				noun = "file"
			}
			c.filesStr = fmt.Sprintf("~%d %s", st.uncommittedCount, noun)
			maxFilesW = max(maxFilesW, len(c.filesStr))
		}
		if st.loaded && st.linesAdded > 0 {
			c.addedStr = fmt.Sprintf("+%d", st.linesAdded)
			maxAddedW = max(maxAddedW, len(c.addedStr))
		}
		if st.loaded && st.linesRemoved > 0 {
			c.removedStr = fmt.Sprintf("-%d", st.linesRemoved)
			maxRemovedW = max(maxRemovedW, len(c.removedStr))
		}
		if st.loaded && st.age != "" {
			c.ageStr = st.age
			maxAgeW = max(maxAgeW, len(c.ageStr))
		}
		c.prLoaded = st.prLoaded
		c.prFound = st.prFound
		c.prNumber = st.prNumber
		c.prState = st.prState
	}

	sp := func(n int) string {
		if n <= 0 {
			return ""
		}
		return strings.Repeat(" ", n)
	}

	// optCol renders an optional column with consistent width across rows.
	// Returns "" when the column is unused for the whole table. When `right`
	// is true the value is right-aligned within colW, otherwise left-aligned.
	optCol := func(prefix string, value string, colW int, style lipgloss.Style, right bool) string {
		if colW == 0 {
			return ""
		}
		styled := ""
		if value != "" {
			styled = style.Render(value)
		}
		pad := sp(colW - len(value))
		if right {
			return prefix + pad + styled
		}
		return prefix + styled + pad
	}

	// pass 2: render each row with consistent column widths
	var b strings.Builder
	b.WriteString(m.prompt)
	b.WriteString("\n")

	for i, c := range cells {
		prefix := "    "
		if i == m.cursor {
			prefix = "  " + styleArrow.Render("▸") + " "
		}

		// name column (left-aligned; "(current)" folds into the width)
		nameCell := styleName.Render(c.name)
		nameW := len(c.name)
		if c.isCurrent {
			nameCell += styleCurrent.Render(" (current)")
			nameW += len(" (current)")
		}
		nameCell += sp(maxNameW - nameW)

		// branch column "(branch)" — padding after the closing paren
		branchCell := styleParens.Render("(") + styleBranch.Render(c.branch) + styleParens.Render(")")
		branchCell += sp(maxBranchW - len(c.branch))

		filesCell := optCol("  ", c.filesStr, maxFilesW, styleDirty, false)
		addedCell := optCol("  ", c.addedStr, maxAddedW, styleAdded, true)
		removedCell := optCol(" ", c.removedStr, maxRemovedW, styleRemoved, true)
		ageCell := optCol("  ", c.ageStr, maxAgeW, styleAge, false)

		// PR column (no width padding — last column)
		var prCell string
		if c.prLoaded && c.prFound {
			switch c.prState {
			case "MERGED":
				prCell = "  " + styleMerged.Render("✓ merged")
			case "OPEN":
				prCell = "  " + styleOpenPR.Render(fmt.Sprintf("#%d", c.prNumber))
			case "CLOSED":
				prCell = "  " + styleClosedPR.Render("✗ closed")
			}
		}

		// behind-origin indicator (main row only, once the async fetch lands)
		var behindCell string
		if i == m.mainIndex && m.behindLoaded && m.behindCount > 0 {
			behindCell = "  " + styleDirty.Render(fmt.Sprintf("↓%d behind origin/%s", m.behindCount, m.defaultBranch))
		}

		row := prefix + nameCell + "  " + branchCell + filesCell + addedCell + removedCell + ageCell + prCell + behindCell

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

	if m.flashMsg != "" {
		b.WriteString(styleFlash.Render("  " + m.flashMsg))
	} else {
		keys := []string{
			fk("↑/↓ j/k", "navigate"),
			fk("enter", string(m.defAction)),
			fk("x", "remove"),
			fk("D", "remove merged"),
		}
		if m.mainIndex >= 0 && m.behindLoaded && m.behindCount > 0 {
			keys = append(keys, fk("p", "pull origin/"+m.defaultBranch))
		}
		keys = append(keys,
			fk("e", "local config"),
			fk("g", "global config"),
			fk("q/esc", "quit"),
		)
		b.WriteString("  " + strings.Join(keys, sep))
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
