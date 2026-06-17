// Package picker is the Bubble Tea TUI worktree picker.
package picker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/McBrideMusings/wtree/internal/gh"
	"github.com/McBrideMusings/wtree/internal/gitwt"
	"github.com/McBrideMusings/wtree/internal/slug"
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
	ActionAddPR // check out a review-inbox PR as a new worktree
)

type Selection struct {
	Action    Action
	Worktree  gitwt.Worktree
	Worktrees []gitwt.Worktree // populated for ActionRemoveMerged
	PRNumber  int              // populated for ActionAddPR
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
// worktree out themselves) and returns what the user picked. nwo is the repo's
// "owner/repo" used to build hyperlinks; when dashboard is true the worktrees are
// grouped by status and the "needs my review" inbox is loaded.
func Run(ctx context.Context, prompt string, defAction DefaultAction, list []gitwt.Worktree, currentPath, mainPath, defaultBranch, nwo string, dashboard bool) (Selection, error) {
	if len(list) == 0 {
		return Selection{}, fmt.Errorf("no worktrees")
	}
	m := newModel(ctx, prompt, defAction, list, currentPath, mainPath, defaultBranch, nwo, dashboard)
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
	case ActionAddPR:
		return Selection{Action: ActionAddPR, PRNumber: fm.addPRNumber}, nil
	case ActionRemoveMerged:
		wts := make([]gitwt.Worktree, len(fm.mergedToRemove))
		for i, idx := range fm.mergedToRemove {
			wts[i] = fm.list[idx]
		}
		return Selection{Action: ActionRemoveMerged, Worktrees: wts}, nil
	default:
		return Selection{Action: fm.action, Worktree: fm.list[fm.selectedWtIndex]}, nil
	}
}

type statusMsg struct {
	index            int
	dirty            bool
	uncommittedCount int
	linesAdded       int
	linesRemoved     int
	aheadCount       int
	age              string
}

type prStatusMsg struct {
	index  int
	number int
	state  string
	found  bool
	ghErr  bool // gh binary missing or auth failure
}

type issueMsg struct {
	index  int
	number int
	fromPR bool
}

type inboxMsg struct {
	prs   []gh.InboxPR
	ghErr bool
}

type behindMsg struct {
	count int
	err   bool
}

type spinnerTickMsg struct{}

type flashResultMsg struct{ text string }

type model struct {
	ctx             context.Context
	prompt          string
	defAction       DefaultAction
	list            []gitwt.Worktree
	currentPath     string
	mainPath        string // primary worktree path; "" when no main row is shown
	mainIndex       int    // index of the main row in list, or -1
	defaultBranch   string // branch compared against origin for the behind check
	nwo             string // owner/repo for hyperlinks
	dashboard       bool   // group by status + show review inbox
	cursor          int    // ordinal over selectable items
	selectedWtIndex int    // resolved worktree index for the final selection
	width           int
	status          []rowStatus
	behindLoaded    bool
	behindCount     int
	inbox           []gh.InboxPR
	inboxLoaded     bool
	inboxErr        bool
	spinnerFrame    int
	action          Action
	addPRNumber     int
	finished        bool
	mode            viewMode
	mergedToRemove  []int
	flashMsg        string
	ghUnavailable   bool
}

type rowStatus struct {
	loaded           bool
	dirty            bool
	uncommittedCount int
	linesAdded       int
	linesRemoved     int
	aheadCount       int
	age              string
	prLoaded         bool
	prFound          bool
	prNumber         int
	prState          string
	issueNum         int
	issueFromPR      bool
}

var (
	styleSelected     = lipgloss.NewStyle().Reverse(true)
	styleFooter       = lipgloss.NewStyle().Faint(true)
	styleFooterKey    = lipgloss.NewStyle().Foreground(lipgloss.Color("6")) // cyan keys
	styleDirty        = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	styleCurrent      = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	styleAge          = lipgloss.NewStyle().Faint(true)
	styleBranch       = lipgloss.NewStyle().Foreground(lipgloss.Color("6")) // cyan
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
	styleSection      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4")) // blue bold
	styleIssue        = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))            // magenta
	styleLink         = lipgloss.NewStyle().Faint(true)
	styleNotReviewed  = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	styleUpdated      = lipgloss.NewStyle().Foreground(lipgloss.Color("6")) // cyan
	styleAuthor       = lipgloss.NewStyle().Faint(true)
	styleUpToDate     = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	styleSpinner      = lipgloss.NewStyle().Foreground(lipgloss.Color("6")) // cyan
)

func newModel(ctx context.Context, prompt string, defAction DefaultAction, list []gitwt.Worktree, currentPath, mainPath, defaultBranch, nwo string, dashboard bool) model {
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
		nwo:           nwo,
		dashboard:     dashboard,
		status:        make([]rowStatus, len(list)),
	}
}

func (m model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.list)*3+3)
	for i, w := range m.list {
		cmds = append(cmds, fetchStatus(m.ctx, i, w.Path, m.defaultBranch))
		cmds = append(cmds, fetchPRStatus(m.ctx, i, w.Branch))
		if m.dashboard {
			cmds = append(cmds, fetchHeuristicIssue(m.ctx, i, w.Branch))
		}
	}
	if m.mainIndex >= 0 && m.defaultBranch != "" {
		cmds = append(cmds, fetchBehind(m.ctx, m.mainPath, m.defaultBranch))
	}
	if m.dashboard {
		branches := make([]string, 0, len(m.list))
		for _, w := range m.list {
			if w.Branch != "" {
				branches = append(branches, w.Branch)
			}
		}
		cmds = append(cmds, fetchInbox(m.ctx, m.nwo, branches), spinnerTick())
	}
	return tea.Batch(cmds...)
}

func spinnerTick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return spinnerTickMsg{} })
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

// fetchLinkedIssue resolves the issue a PR closes (authoritative association).
func fetchLinkedIssue(parent context.Context, index, prNumber int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 10*time.Second)
		defer cancel()
		num, found, err := gh.LinkedIssue(ctx, prNumber)
		if err != nil || !found {
			return issueMsg{index: index, fromPR: true} // number 0 → no override
		}
		return issueMsg{index: index, number: num, fromPR: true}
	}
}

// fetchHeuristicIssue guesses the issue from a "<num>-slug" branch name and
// verifies it exists before reporting it. Only used as a fallback when a
// worktree has no PR-declared issue.
func fetchHeuristicIssue(parent context.Context, index int, branch string) tea.Cmd {
	return func() tea.Msg {
		num, ok := slug.IssueNumberFromBranch(branch)
		if !ok {
			return issueMsg{index: index}
		}
		ctx, cancel := context.WithTimeout(parent, 10*time.Second)
		defer cancel()
		if !gh.IssueExists(ctx, num) {
			return issueMsg{index: index}
		}
		return issueMsg{index: index, number: num}
	}
}

func fetchInbox(parent context.Context, nwo string, localBranches []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 15*time.Second)
		defer cancel()
		prs, err := gh.ReviewInbox(ctx, nwo, localBranches)
		if err != nil {
			return inboxMsg{ghErr: true}
		}
		return inboxMsg{prs: prs}
	}
}

func fetchStatus(parent context.Context, index int, path, defaultBranch string) tea.Cmd {
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

		aheadCount := 0
		if defaultBranch != "" {
			s := git("rev-list", "--count", defaultBranch+"..HEAD")
			if s == "" {
				s = git("rev-list", "--count", "origin/"+defaultBranch+"..HEAD")
			}
			aheadCount, _ = strconv.Atoi(s)
		}

		return statusMsg{
			index:            index,
			dirty:            uncommittedCount > 0,
			uncommittedCount: uncommittedCount,
			linesAdded:       linesAdded,
			linesRemoved:     linesRemoved,
			aheadCount:       aheadCount,
			age:              git("log", "-1", "--format=%cr", "HEAD"),
		}
	}
}

func openInBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		if url == "" {
			return flashResultMsg{"Nothing to open for this row."}
		}
		opener := "xdg-open"
		if runtime.GOOS == "darwin" {
			opener = "open"
		}
		if err := exec.Command(opener, url).Start(); err != nil {
			return flashResultMsg{"Couldn't open browser: " + err.Error()}
		}
		return flashResultMsg{"Opened in browser."}
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
			st.aheadCount = msg.aheadCount
			st.age = msg.age
		}
		return m, nil
	case prStatusMsg:
		if msg.index >= 0 && msg.index < len(m.status) {
			st := &m.status[msg.index]
			st.prLoaded = true // set even on error so the categorize gate completes
			if msg.ghErr {
				m.ghUnavailable = true
				return m, nil
			}
			st.prFound = msg.found
			st.prNumber = msg.number
			st.prState = msg.state
			if m.dashboard && msg.found && msg.number > 0 {
				return m, fetchLinkedIssue(m.ctx, msg.index, msg.number)
			}
		}
		return m, nil
	case issueMsg:
		if msg.index >= 0 && msg.index < len(m.status) {
			st := &m.status[msg.index]
			// A PR-declared issue is authoritative and always wins; a heuristic
			// guess only applies when no PR-declared issue has landed.
			if msg.fromPR {
				st.issueFromPR = true
				if msg.number > 0 {
					st.issueNum = msg.number
				}
			} else if !st.issueFromPR && msg.number > 0 {
				st.issueNum = msg.number
			}
		}
		return m, nil
	case inboxMsg:
		m.inboxLoaded = true
		m.inboxErr = msg.ghErr
		m.inbox = msg.prs
		return m, nil
	case behindMsg:
		m.behindLoaded = !msg.err
		if !msg.err {
			m.behindCount = msg.count
		}
		return m, nil
	case spinnerTickMsg:
		m.spinnerFrame++
		if m.loading() {
			return m, spinnerTick()
		}
		return m, nil
	case flashResultMsg:
		m.flashMsg = msg.text
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
			return m, nil
		}

		// While the dashboard is still loading, only quit keys are live — no
		// navigation or destructive action runs on a partial dataset.
		if m.dashboard && m.loading() {
			if s := msg.String(); s == "ctrl+c" || s == "q" || s == "esc" {
				m.action = ActionNone
				m.finished = true
				return m, tea.Quit
			}
			return m, nil
		}

		items := m.selectableItems()
		if len(items) == 0 {
			if s := msg.String(); s == "ctrl+c" || s == "q" || s == "esc" {
				m.action = ActionNone
				m.finished = true
				return m, tea.Quit
			}
			return m, nil
		}
		if m.cursor >= len(items) {
			m.cursor = len(items) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		cur := items[m.cursor]

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
			if m.cursor < len(items)-1 {
				m.cursor++
			}
		case "enter":
			if cur.isInbox {
				m.action = ActionAddPR
				m.addPRNumber = m.inbox[cur.idx].Number
				m.finished = true
				return m, tea.Quit
			}
			if m.defAction == DefaultRemove {
				if cur.idx == m.mainIndex {
					m.flashMsg = "Cannot remove the primary worktree."
					break
				}
				m.action = ActionRemove
			} else {
				m.action = ActionEnter
			}
			m.selectedWtIndex = cur.idx
			m.finished = true
			return m, tea.Quit
		case "x":
			if cur.isInbox {
				m.flashMsg = "Nothing to remove — that's a review PR, not a worktree."
				break
			}
			if cur.idx == m.mainIndex {
				m.flashMsg = "Cannot remove the primary worktree."
				break
			}
			m.action = ActionRemove
			m.selectedWtIndex = cur.idx
			m.finished = true
			return m, tea.Quit
		case "o":
			return m, openInBrowser(m.openURLFor(cur))
		case "i":
			url := m.issueURLFor(cur)
			if url == "" {
				m.flashMsg = "No issue associated with this row."
				break
			}
			return m, openInBrowser(url)
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
	return m, nil
}

// selItem identifies one selectable row: a worktree (by list index) or an inbox
// PR (by inbox index).
type selItem struct {
	isInbox bool
	idx     int
}

// worktreesReady reports whether every worktree has both its git status and PR
// status resolved — the point at which buckets are final.
func (m model) worktreesReady() bool {
	for i := range m.status {
		if !m.status[i].loaded || !m.status[i].prLoaded {
			return false
		}
	}
	return true
}

// loading reports whether anything is still resolving (drives the spinner ticks).
func (m model) loading() bool {
	if !m.worktreesReady() {
		return true
	}
	if !m.dashboard {
		return false
	}
	return !m.inboxLoaded
}

// buckets groups worktree indices by status (0..4: primary, in-review,
// merged/cleanup, in-progress, idle).
func (m model) buckets() [5][]int {
	var bs [5][]int
	for i := range m.list {
		b := m.bucketOf(i)
		bs[b] = append(bs[b], i)
	}
	return bs
}

// bucketOf returns the status bucket index for worktree i.
func (m model) bucketOf(i int) int {
	if i == m.mainIndex {
		return 0
	}
	st := m.status[i]
	if st.prLoaded && st.prFound {
		switch st.prState {
		case "OPEN":
			return 1
		case "MERGED", "CLOSED":
			return 2
		}
	}
	if !st.loaded || st.dirty || st.aheadCount > 0 {
		return 3
	}
	return 4
}

// selectableItems is the ordered list the cursor moves over: worktrees (only
// once categorized) followed by the review PRs that need action.
func (m model) selectableItems() []selItem {
	var items []selItem
	if !m.dashboard {
		for i := range m.list {
			items = append(items, selItem{idx: i})
		}
		return items
	}
	if m.worktreesReady() {
		bs := m.buckets()
		for b := range 5 {
			for _, i := range bs[b] {
				items = append(items, selItem{idx: i})
			}
		}
	}
	for j := range m.inbox {
		items = append(items, selItem{isInbox: true, idx: j})
	}
	return items
}

func (m model) openURLFor(r selItem) string {
	if r.isInbox {
		return gh.PRURL(m.nwo, m.inbox[r.idx].Number)
	}
	st := m.status[r.idx]
	if st.prFound && st.prNumber > 0 {
		return gh.PRURL(m.nwo, st.prNumber)
	}
	if st.issueNum > 0 {
		return gh.IssueURL(m.nwo, st.issueNum)
	}
	return gh.RepoURL(m.nwo)
}

func (m model) issueURLFor(r selItem) string {
	if r.isInbox {
		return ""
	}
	if st := m.status[r.idx]; st.issueNum > 0 {
		return gh.IssueURL(m.nwo, st.issueNum)
	}
	return ""
}

// hyperlink wraps text in an OSC-8 terminal hyperlink. Returns text unchanged
// when url is empty (e.g. no origin remote).
func hyperlink(url, text string) string {
	if url == "" {
		return text
	}
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

// loadingBar renders an indeterminate "marching block" progress bar, animated
// by the spinner tick.
func (m model) loadingBar() string {
	const barW, fillW = 28, 8
	pos := m.spinnerFrame % barW
	var sb strings.Builder
	sb.WriteString(styleParens.Render("["))
	for k := range barW {
		if d := (k - pos + barW) % barW; d < fillW {
			sb.WriteString(styleSpinner.Render("▰"))
		} else {
			sb.WriteString(styleParens.Render("▱"))
		}
	}
	sb.WriteString(styleParens.Render("]"))
	return sb.String()
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
	if !m.dashboard {
		return m.viewFlat(width, fk, sep)
	}
	return m.viewDashboard(width, fk, sep)
}

// cells holds the measured, per-worktree column values shared by both views.
type cells struct {
	name       []string
	branch     []string
	isCurrent  []bool
	filesStr   []string
	addedStr   []string
	removedStr []string
	ageStr     []string
	maxName    int
	maxBranch  int
	maxLabel   int // branch + " (current)" — dashboard's single label column
	maxFiles   int
	maxAdded   int
	maxRemoved int
	maxAge     int
}

func (m model) measure() cells {
	c := cells{
		name:       make([]string, len(m.list)),
		branch:     make([]string, len(m.list)),
		isCurrent:  make([]bool, len(m.list)),
		filesStr:   make([]string, len(m.list)),
		addedStr:   make([]string, len(m.list)),
		removedStr: make([]string, len(m.list)),
		ageStr:     make([]string, len(m.list)),
	}
	for i, w := range m.list {
		c.name[i] = filepath.Base(w.Path)
		c.isCurrent[i] = w.Path == m.currentPath && m.currentPath != ""

		nameW := len(c.name[i])
		if c.isCurrent[i] {
			nameW += len(" (current)")
		}
		c.maxName = max(c.maxName, nameW)

		if w.Detached {
			short := w.Head
			if len(short) > 7 {
				short = short[:7]
			}
			c.branch[i] = "detached " + short
		} else {
			c.branch[i] = w.Branch
		}
		c.maxBranch = max(c.maxBranch, len(c.branch[i]))
		labelW := len(c.branch[i])
		if c.isCurrent[i] {
			labelW += len(" (current)")
		}
		c.maxLabel = max(c.maxLabel, labelW)

		st := m.status[i]
		if st.loaded && st.dirty {
			noun := "files"
			if st.uncommittedCount == 1 {
				noun = "file"
			}
			c.filesStr[i] = fmt.Sprintf("~%d %s", st.uncommittedCount, noun)
			c.maxFiles = max(c.maxFiles, len(c.filesStr[i]))
		}
		if st.loaded && st.linesAdded > 0 {
			c.addedStr[i] = fmt.Sprintf("+%d", st.linesAdded)
			c.maxAdded = max(c.maxAdded, len(c.addedStr[i]))
		}
		if st.loaded && st.linesRemoved > 0 {
			c.removedStr[i] = fmt.Sprintf("-%d", st.linesRemoved)
			c.maxRemoved = max(c.maxRemoved, len(c.removedStr[i]))
		}
		if st.loaded && st.age != "" {
			c.ageStr[i] = st.age
			c.maxAge = max(c.maxAge, len(c.ageStr[i]))
		}
	}
	return c
}

func sp(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}

func optCol(prefix, value string, colW int, style lipgloss.Style, right bool) string {
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

// statusCols renders the files / +N / -N / age columns common to both views.
func (m model) statusCols(c cells, i int) string {
	return optCol("  ", c.filesStr[i], c.maxFiles, styleDirty, false) +
		optCol("  ", c.addedStr[i], c.maxAdded, styleAdded, true) +
		optCol(" ", c.removedStr[i], c.maxRemoved, styleRemoved, true) +
		optCol("  ", c.ageStr[i], c.maxAge, styleAge, false)
}

// prCell renders the PR badge; in dashboard mode it is a hyperlink with a ↗ hint.
func (m model) prCell(i int, link bool) string {
	st := m.status[i]
	if !st.prLoaded || !st.prFound {
		return ""
	}
	var text string
	var style lipgloss.Style
	switch st.prState {
	case "MERGED":
		text, style = fmt.Sprintf("✓ merged #%d", st.prNumber), styleMerged
	case "OPEN":
		text, style = fmt.Sprintf("#%d open", st.prNumber), styleOpenPR
	case "CLOSED":
		text, style = fmt.Sprintf("✗ closed #%d", st.prNumber), styleClosedPR
	default:
		return ""
	}
	rendered := style.Render(text)
	if link {
		rendered = hyperlink(gh.PRURL(m.nwo, st.prNumber), rendered) + styleLink.Render(" ↗")
	}
	return "  " + rendered
}

func (m model) row(body string, isCursor bool, width int) string {
	prefix := "    "
	if isCursor {
		prefix = "  " + styleArrow.Render("▸") + " "
	}
	r := prefix + body
	if isCursor {
		if pad := width - lipgloss.Width(r); pad > 0 {
			r += strings.Repeat(" ", pad)
		}
		r = styleSelected.Render(r)
	}
	return r
}

// ---- flat view (the `wtree rm` picker) — unchanged behavior ----

func (m model) viewFlat(width int, fk func(string, string) string, sep string) string {
	c := m.measure()
	var b strings.Builder
	b.WriteString(m.prompt)
	b.WriteString("\n")

	for i := range m.list {
		nameCell := styleName.Render(c.name[i])
		nameW := len(c.name[i])
		if c.isCurrent[i] {
			nameCell += styleCurrent.Render(" (current)")
			nameW += len(" (current)")
		}
		nameCell += sp(c.maxName - nameW)

		branchCell := styleParens.Render("(") + styleBranch.Render(c.branch[i]) + styleParens.Render(")")
		branchCell += sp(c.maxBranch - len(c.branch[i]))

		body := nameCell + "  " + branchCell + m.statusCols(c, i) + m.prCell(i, false)
		b.WriteString(m.row(body, i == m.cursor, width))
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
			fk("e", "local config"),
			fk("g", "global config"),
			fk("q/esc", "quit"),
		}
		b.WriteString("  " + strings.Join(keys, sep))
	}
	return b.String()
}

// ---- dashboard view ----

func (m model) viewLoading() string {
	var b strings.Builder
	b.WriteString(m.prompt)
	b.WriteString("\n\n")
	b.WriteString("  " + m.loadingBar() + styleFooter.Render("  loading your work…"))
	b.WriteString("\n")
	return b.String()
}

func (m model) viewDashboard(width int, fk func(string, string) string, sep string) string {
	if m.loading() {
		return m.viewLoading()
	}
	c := m.measure()
	items := m.selectableItems()
	cursorOK := len(items) > 0
	cur := m.cursor
	if cursorOK {
		if cur >= len(items) {
			cur = len(items) - 1
		}
		if cur < 0 {
			cur = 0
		}
	}
	var cursorItem selItem
	if cursorOK {
		cursorItem = items[cur]
	}
	isCursor := func(it selItem) bool { return cursorOK && cursorItem == it }

	var b strings.Builder
	b.WriteString(m.prompt)
	b.WriteString("\n")
	if m.nwo != "" {
		url := gh.RepoURL(m.nwo)
		b.WriteString(styleName.Render("  "+m.nwo) + "  " + hyperlink(url, styleLink.Render(url+" ↗")) + "\n")
	}

	labels := [5]string{"PRIMARY", "IN REVIEW", "MERGED · cleanup", "IN PROGRESS", "IDLE"}
	bs := m.buckets()
	for bIdx := range 5 {
		if len(bs[bIdx]) == 0 {
			continue
		}
		b.WriteString("\n")
		b.WriteString(styleSection.Render("  "+labels[bIdx]) + "\n")
		for _, i := range bs[bIdx] {
			body := m.worktreeBody(c, i)
			b.WriteString(m.row(body, isCursor(selItem{idx: i}), width))
			b.WriteString("\n")
		}
	}

	m.writeInbox(&b, width, isCursor)

	if m.flashMsg != "" {
		b.WriteString(styleFlash.Render("  " + m.flashMsg))
	} else {
		b.WriteString("  " + m.dashboardFooter(fk, sep))
	}
	return b.String()
}

// worktreeBody renders a dashboard worktree row: a single branch label (no
// directory-name column), status columns, the PR link, an optional issue link,
// and the behind/up-to-date marker on the primary row.
func (m model) worktreeBody(c cells, i int) string {
	label := c.branch[i]
	labelCell := styleBranch.Render(label)
	labelW := len(label)
	if c.isCurrent[i] {
		labelCell += styleCurrent.Render(" (current)")
		labelW += len(" (current)")
	}
	labelCell += sp(c.maxLabel - labelW)

	body := labelCell + m.statusCols(c, i) + m.prCell(i, true)

	if st := m.status[i]; st.issueNum > 0 {
		text := styleIssue.Render(fmt.Sprintf("issue #%d", st.issueNum))
		body += "  " + hyperlink(gh.IssueURL(m.nwo, st.issueNum), text) + styleLink.Render(" ↗")
	}

	if i == m.mainIndex {
		switch {
		case m.behindLoaded && m.behindCount > 0:
			body += "  " + styleDirty.Render(fmt.Sprintf("↓%d behind origin/%s", m.behindCount, m.defaultBranch))
		case m.behindLoaded && m.behindCount == 0:
			body += "  " + styleUpToDate.Render("✓ up to date")
		}
	}
	return body
}

// writeInbox renders the NEEDS MY REVIEW section. By the time the dashboard
// renders, the inbox is fully loaded (the loading bar gates on that) and
// ReviewInbox has already dropped reviewed-current PRs, so every entry is shown.
func (m model) writeInbox(b *strings.Builder, width int, isCursor func(selItem) bool) {
	if m.inboxErr {
		b.WriteString("\n")
		b.WriteString(styleFooter.Render("  (review inbox unavailable — gh not found or not authenticated)") + "\n")
		return
	}
	if len(m.inbox) == 0 {
		return // nothing to review — collapse the section entirely
	}

	maxNumW, maxAuthorW := 0, 0
	for _, pr := range m.inbox {
		maxNumW = max(maxNumW, len(fmt.Sprintf("#%d", pr.Number)))
		maxAuthorW = max(maxAuthorW, len(pr.Author))
	}

	rule := strings.Repeat("─", min(width-2, 74))
	b.WriteString(styleFooter.Render("  "+rule) + "\n") // rule hugs the section above it
	b.WriteString(styleSection.Render(fmt.Sprintf("  NEEDS MY REVIEW · %d", len(m.inbox))) + "\n")

	for j := range m.inbox {
		body := m.inboxBody(j, maxNumW, maxAuthorW)
		b.WriteString(m.row(body, isCursor(selItem{isInbox: true, idx: j}), width))
		b.WriteString("\n")
	}
	b.WriteString("\n") // blank line between the last review row and the footer
}

func (m model) inboxBody(j, maxNumW, maxAuthorW int) string {
	pr := m.inbox[j]
	numText := fmt.Sprintf("#%d", pr.Number)
	numCell := hyperlink(gh.PRURL(m.nwo, pr.Number), styleOpenPR.Render(numText)) + styleLink.Render(" ↗")
	numCell += sp(maxNumW - len(numText))

	title := truncate(pr.Title, 44)
	titleCell := styleName.Render(title) + sp(44-len([]rune(title)))

	authorCell := styleAuthor.Render("@"+pr.Author) + sp(maxAuthorW-len(pr.Author))

	ageCell := styleAge.Render("updated " + relTime(pr.Updated))

	var statusCell string
	switch pr.State {
	case gh.NotReviewed:
		statusCell = styleNotReviewed.Render("● not reviewed")
	case gh.UpdatedSinceReview:
		statusCell = styleUpdated.Render("↻ updated since your review")
	}

	return numCell + "  " + titleCell + "  " + authorCell + "  " + ageCell + "  " + statusCell
}

func (m model) dashboardFooter(fk func(string, string) string, sep string) string {
	keys := []string{
		fk("↑/↓ j/k", "navigate"),
		fk("enter", "cd / check out"),
		fk("x", "remove"),
		fk("D", "remove merged"),
	}
	if m.mainIndex >= 0 && m.behindLoaded && m.behindCount > 0 {
		keys = append(keys, fk("p", "pull origin/"+m.defaultBranch))
	}
	keys = append(keys,
		fk("o", "open ↗"),
		fk("i", "issue ↗"),
		fk("e", "local config"),
		fk("g", "global config"),
		fk("q/esc", "quit"),
	)
	return strings.Join(keys, sep)
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

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

func relTime(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dw", int(d.Hours()/24/7))
	}
}
