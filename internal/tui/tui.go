package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cjhargre/mainstacks/internal/agent"
	"github.com/cjhargre/mainstacks/internal/gemini"
	"github.com/cjhargre/mainstacks/internal/skill"
)

type view int

const (
	viewMenu view = iota
	viewIngesting
	viewBrowse
	viewDetail
	viewWrite
	viewWriteDone
)

type category struct {
	name   string
	skills []int // indices into m.skills
}

type Model struct {
	view       view
	cursor     int
	browseCur  int
	writeCur   int
	selected   map[int]bool
	spinner    spinner.Model
	client     *gemini.Client
	store      *skill.SQLiteStore
	err        error
	loading    bool
	skills     []skill.Skill
	groups     []category
	flatIndex  []int // maps flat cursor position → skill index (-1 for headers)
}

type ingestDoneMsg struct {
	skills []skill.Skill
	err    error
}

type writeDoneMsg struct{ err error }

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	selectedStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
	itemStyle     = lipgloss.NewStyle().PaddingLeft(2)
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	tagStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	groupStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).PaddingLeft(2)
)

func New(client *gemini.Client, store *skill.SQLiteStore) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return Model{view: viewMenu, spinner: sp, client: client, store: store}
}

func (m *Model) buildGroups() {
	catMap := map[string][]int{
		"Patterns":       {},
		"Infrastructure": {},
		"Operations":     {},
		"Design":         {},
	}

	for i, sk := range m.skills {
		switch sk.Type {
		case skill.TypeCode, skill.TypeProto:
			catMap["Patterns"] = append(catMap["Patterns"], i)
		case skill.TypeInfra, skill.TypeTerraform:
			catMap["Infrastructure"] = append(catMap["Infrastructure"], i)
		case skill.TypeRunbook:
			catMap["Operations"] = append(catMap["Operations"], i)
		default:
			catMap["Design"] = append(catMap["Design"], i)
		}
	}

	m.groups = nil
	m.flatIndex = nil
	order := []string{"Patterns", "Infrastructure", "Operations", "Design"}
	for _, name := range order {
		indices := catMap[name]
		if len(indices) == 0 {
			continue
		}
		m.groups = append(m.groups, category{name: name, skills: indices})
		m.flatIndex = append(m.flatIndex, -1) // header
		for _, idx := range indices {
			m.flatIndex = append(m.flatIndex, idx)
		}
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.view == viewMenu {
				return m, tea.Quit
			}
			if m.view == viewDetail {
				m.view = viewBrowse
				return m, nil
			}
			m.view = viewMenu
			m.err = nil
			m.loading = false
			return m, nil
		case "esc":
			if m.view == viewDetail {
				m.view = viewBrowse
				return m, nil
			}
			m.view = viewMenu
			m.err = nil
			m.loading = false
			return m, nil
		}
	}

	switch m.view {
	case viewMenu:
		return m.updateMenu(msg)
	case viewIngesting:
		return m.updateIngesting(msg)
	case viewBrowse:
		return m.updateBrowse(msg)
	case viewWrite:
		return m.updateWrite(msg)
	case viewDetail:
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) updateMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < 3 {
				m.cursor++
			}
		case "enter":
			switch m.cursor {
			case 0:
				m.view = viewIngesting
				m.loading = true
				m.skills = nil
				m.err = nil
				return m, tea.Batch(m.spinner.Tick, m.doIngest())
			case 1:
				m.view = viewWrite
				m.skills = m.store.All()
				m.buildGroups()
				m.writeCur = 0
				m.skipToNextSkill(&m.writeCur, 1)
				m.selected = make(map[int]bool)
				return m, nil
			case 2:
				m.view = viewBrowse
				m.skills = m.store.All()
				m.buildGroups()
				m.browseCur = 0
				m.skipToNextSkill(&m.browseCur, 1)
				return m, nil
			case 3:
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m Model) updateIngesting(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ingestDoneMsg:
		m.loading = false
		m.err = msg.err
		m.skills = msg.skills
		return m, nil
	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m *Model) skipToNextSkill(cur *int, dir int) {
	for *cur >= 0 && *cur < len(m.flatIndex) && m.flatIndex[*cur] == -1 {
		*cur += dir
	}
	if *cur < 0 {
		*cur = 0
		m.skipToNextSkill(cur, 1)
	}
	if *cur >= len(m.flatIndex) {
		*cur = len(m.flatIndex) - 1
	}
}

func (m Model) updateBrowse(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.browseCur > 0 {
				m.browseCur--
				m.skipToNextSkill(&m.browseCur, -1)
			}
		case "down", "j":
			if m.browseCur < len(m.flatIndex)-1 {
				m.browseCur++
				m.skipToNextSkill(&m.browseCur, 1)
			}
		case "enter":
			if len(m.flatIndex) > 0 && m.flatIndex[m.browseCur] >= 0 {
				m.view = viewDetail
			}
			return m, nil
		case "d":
			if len(m.flatIndex) > 0 && m.flatIndex[m.browseCur] >= 0 {
				idx := m.flatIndex[m.browseCur]
				m.store.Delete(m.skills[idx].Name)
				m.skills = m.store.All()
				m.buildGroups()
				if m.browseCur >= len(m.flatIndex) {
					m.browseCur = len(m.flatIndex) - 1
				}
				m.skipToNextSkill(&m.browseCur, -1)
			}
			return m, nil
		}
	}
	return m, nil
}

func (m Model) updateWrite(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.writeCur > 0 {
				m.writeCur--
				m.skipToNextSkill(&m.writeCur, -1)
			}
		case "down", "j":
			if m.writeCur < len(m.flatIndex)-1 {
				m.writeCur++
				m.skipToNextSkill(&m.writeCur, 1)
			}
		case " ":
			if m.flatIndex[m.writeCur] >= 0 {
				idx := m.flatIndex[m.writeCur]
				m.selected[idx] = !m.selected[idx]
			}
		case "enter":
			var picked []skill.Skill
			for idx, sel := range m.selected {
				if sel {
					picked = append(picked, m.skills[idx])
				}
			}
			if len(picked) == 0 {
				m.err = fmt.Errorf("no skills selected")
				return m, nil
			}
			m.err = writeSkillsMD(picked)
			m.view = viewWriteDone
			return m, nil
		}
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("⚡ mainstacks") + dimStyle.Render(fmt.Sprintf(" (%d skills in library)", m.store.Count())) + "\n\n")

	switch m.view {
	case viewMenu:
		items := []string{"Ingest this repo", "Write skills", "Browse skills", "Quit"}
		for i, item := range items {
			if i == m.cursor {
				b.WriteString(selectedStyle.Render("→ "+item) + "\n")
			} else {
				b.WriteString(itemStyle.Render("  "+item) + "\n")
			}
		}
		b.WriteString("\n" + dimStyle.Render("j/k to move, enter to select, q to quit"))

	case viewIngesting:
		if m.loading {
			b.WriteString("  " + m.spinner.View() + " Analyzing codebase...\n")
		} else if m.err != nil {
			b.WriteString("  " + errorStyle.Render("Error: "+m.err.Error()) + "\n")
		} else {
			b.WriteString(successStyle.Render(fmt.Sprintf("  ✓ Extracted %d skills:\n\n", len(m.skills))))
			for _, sk := range m.skills {
				b.WriteString(fmt.Sprintf("  • %s [%s]\n", sk.Name, string(sk.Type)))
			}
		}
		b.WriteString("\n" + dimStyle.Render("esc to go back"))

	case viewBrowse:
		if len(m.skills) == 0 {
			b.WriteString("  No skills in library yet. Ingest a repo first.\n")
		} else {
			b.WriteString(m.renderGroupedList(m.browseCur, false))
			b.WriteString("\n" + dimStyle.Render("j/k to move, enter to view, d to delete, esc to go back"))
		}

	case viewDetail:
		idx := m.flatIndex[m.browseCur]
		sk := m.skills[idx]
		b.WriteString(headerStyle.Render("  "+sk.Name) + "\n\n")
		b.WriteString(fmt.Sprintf("  Type:    %s\n", string(sk.Type)))
		b.WriteString(fmt.Sprintf("  Source:  %s\n", sk.Source))
		if len(sk.Tags) > 0 {
			b.WriteString(fmt.Sprintf("  Tags:    %s\n", tagStyle.Render(strings.Join(sk.Tags, ", "))))
		}
		if len(sk.Dependencies) > 0 {
			b.WriteString(fmt.Sprintf("  Deps:    %s\n", strings.Join(sk.Dependencies, ", ")))
		}
		b.WriteString("\n")
		b.WriteString(wrapIndent(sk.Summary, "  ", 72))
		if sk.Pattern != "" {
			b.WriteString("\n\n" + dimStyle.Render("  Pattern:") + "\n")
			b.WriteString(wrapIndent(sk.Pattern, "    ", 72))
		}
		if sk.Usage != "" {
			b.WriteString("\n\n" + dimStyle.Render("  Usage:") + "\n")
			b.WriteString(wrapIndent(sk.Usage, "  ", 72))
		}
		b.WriteString("\n\n" + dimStyle.Render("esc to go back"))

	case viewWrite:
		if len(m.skills) == 0 {
			b.WriteString("  No skills in library yet.\n")
		} else {
			b.WriteString("  Select skills to write:\n\n")
			b.WriteString(m.renderGroupedList(m.writeCur, true))
			b.WriteString("\n" + dimStyle.Render("space to select, enter to write, esc to go back"))
		}

	case viewWriteDone:
		if m.err != nil {
			b.WriteString("  " + errorStyle.Render(m.err.Error()) + "\n")
		} else {
			b.WriteString("  " + successStyle.Render("✓ SKILLS.md written to current directory") + "\n")
		}
		b.WriteString("\n" + dimStyle.Render("esc to go back"))
	}

	return b.String()
}

func (m Model) renderGroupedList(cursor int, showCheckbox bool) string {
	var b strings.Builder
	pos := 0
	for _, grp := range m.groups {
		// Header
		b.WriteString(groupStyle.Render(fmt.Sprintf("%s (%d)", grp.name, len(grp.skills))) + "\n")
		pos++
		// Items
		for _, idx := range grp.skills {
			sk := m.skills[idx]
			var prefix string
			if showCheckbox {
				check := "○"
				if m.selected[idx] {
					check = successStyle.Render("●")
				}
				prefix = check + " "
			} else {
				prefix = ""
			}
			name := fmt.Sprintf("%s%s", prefix, sk.Name)
			if pos == cursor {
				b.WriteString("    " + selectedStyle.Render("→ "+name) + "\n")
			} else {
				b.WriteString("      " + itemStyle.Render(name) + "\n")
			}
			pos++
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) doIngest() tea.Cmd {
	return func() tea.Msg {
		cwd, err := os.Getwd()
		if err != nil {
			return ingestDoneMsg{err: err}
		}

		skills, err := agent.IngestRepo(context.Background(), m.client, cwd)
		if err != nil {
			return ingestDoneMsg{err: err}
		}

		for _, sk := range skills {
			m.store.Add(sk)
		}

		return ingestDoneMsg{skills: skills}
	}
}

func writeSkillsMD(skills []skill.Skill) error {
	if len(skills) == 0 {
		return fmt.Errorf("no skills to write")
	}

	var b strings.Builder
	b.WriteString("# Skills\n\n")
	b.WriteString("*Generated by [mainstacks](https://github.com/cjhargre/mainstacks)*\n\n")

	for _, sk := range skills {
		b.WriteString(fmt.Sprintf("## %s\n\n", sk.Name))
		b.WriteString(fmt.Sprintf("- **Type:** %s\n", sk.Type))
		b.WriteString(fmt.Sprintf("- **Source:** `%s`\n", sk.Source))
		if len(sk.Tags) > 0 {
			b.WriteString(fmt.Sprintf("- **Tags:** %s\n", strings.Join(sk.Tags, ", ")))
		}
		if len(sk.Dependencies) > 0 {
			b.WriteString(fmt.Sprintf("- **Dependencies:** %s\n", strings.Join(sk.Dependencies, ", ")))
		}
		b.WriteString(fmt.Sprintf("\n%s\n", sk.Summary))
		if sk.Pattern != "" {
			b.WriteString(fmt.Sprintf("\n**Pattern:**\n\n```\n%s\n```\n", sk.Pattern))
		}
		if sk.Usage != "" {
			b.WriteString(fmt.Sprintf("\n**Usage:** %s\n", sk.Usage))
		}
		b.WriteString("\n---\n\n")
	}

	return os.WriteFile("SKILLS.md", []byte(b.String()), 0644)
}

func wrapIndent(text, indent string, width int) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	var lines []string
	line := indent
	for _, w := range words {
		if len(line)+len(w)+1 > width && line != indent {
			lines = append(lines, line)
			line = indent
		}
		if line == indent {
			line += w
		} else {
			line += " " + w
		}
	}
	if line != indent {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
