package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cjhargre/mainstacks/internal/agent"
	"github.com/cjhargre/mainstacks/internal/gemini"
	"github.com/cjhargre/mainstacks/internal/hub"
	"github.com/cjhargre/mainstacks/internal/skill"
)

type view int

const (
	viewMenu view = iota
	viewIngestInput
	viewIngesting
	viewBrowse
	viewDetail
	viewWrite
	viewWriteDone
	viewCommunity     // community market sub-menu
	viewCommunityList // browsing community skills
	viewCommunitySearch
	viewCommunityDetail
	viewUploadSelect // pick a local skill to upload
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
	browsesel  map[int]bool
	confirming bool
	input      textinput.Model
	spinner    spinner.Model
	client     *gemini.Client
	store      *skill.SQLiteStore
	hub        *hub.Client
	err        error
	loading    bool
	skills     []skill.Skill
	groups     []category
	flatIndex  []int // maps flat cursor position → skill index (-1 for headers)
	// community market
	commCur    int
	commSkills []hub.CommunitySkill
	commMsg    string
}

type ingestDoneMsg struct {
	skills []skill.Skill
	err    error
}

type writeDoneMsg struct{ err error }

type communityBrowseMsg struct {
	skills []hub.CommunitySkill
	err    error
}

type communityPublishMsg struct{ err error }

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

func New(client *gemini.Client, store *skill.SQLiteStore, hubClient *hub.Client) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	ti := textinput.New()
	ti.Placeholder = "path (or enter for current directory)"
	return Model{view: viewMenu, spinner: sp, input: ti, client: client, store: store, hub: hubClient}
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
			if m.view == viewCommunityDetail {
				m.view = viewCommunityList
				return m, nil
			}
			if m.view == viewCommunityList {
				m.view = viewCommunity
				m.commMsg = ""
				return m, nil
			}
			if m.view == viewCommunitySearch {
				m.view = viewCommunity
				m.commMsg = ""
				return m, nil
			}
			if m.view == viewCommunity || m.view == viewUploadSelect {
				m.view = viewMenu
				m.commMsg = ""
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
			if m.view == viewCommunityDetail {
				m.view = viewCommunityList
				return m, nil
			}
			if m.view == viewCommunityList {
				m.view = viewCommunity
				m.commMsg = ""
				return m, nil
			}
			if m.view == viewCommunitySearch {
				m.view = viewCommunity
				m.commMsg = ""
				return m, nil
			}
			if m.view == viewCommunity || m.view == viewUploadSelect {
				m.view = viewMenu
				m.commMsg = ""
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
	case viewIngestInput:
		return m.updateIngestInput(msg)
	case viewIngesting:
		return m.updateIngesting(msg)
	case viewBrowse:
		return m.updateBrowse(msg)
	case viewWrite:
		return m.updateWrite(msg)
	case viewCommunity:
		return m.updateCommunity(msg)
	case viewCommunitySearch:
		return m.updateCommunitySearch(msg)
	case viewCommunityList:
		return m.updateCommunityList(msg)
	case viewCommunityDetail:
		return m.updateCommunityDetail(msg)
	case viewUploadSelect:
		return m.updateUploadSelect(msg)
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
			if m.cursor < 4 {
				m.cursor++
			}
		case "enter":
			switch m.cursor {
			case 0:
				m.view = viewIngestInput
				m.input.SetValue("")
				m.input.Focus()
				return m, textinput.Blink
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
				m.browsesel = make(map[int]bool)
				m.confirming = false
				m.skipToNextSkill(&m.browseCur, 1)
				return m, nil
			case 3:
				m.view = viewCommunity
				m.commCur = 0
				m.err = nil
				return m, nil
			case 4:
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m Model) updateIngestInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "enter" {
			path := m.input.Value()
			if path == "" {
				path, _ = os.Getwd()
			}
			m.view = viewIngesting
			m.loading = true
			m.skills = nil
			m.err = nil
			return m, tea.Batch(m.spinner.Tick, m.doIngestPath(path))
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
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
		if m.confirming {
			switch msg.String() {
			case "y":
				for idx, sel := range m.browsesel {
					if sel {
						m.store.Delete(m.skills[idx].Name)
					}
				}
				m.skills = m.store.All()
				m.buildGroups()
				m.browsesel = make(map[int]bool)
				m.confirming = false
				if m.browseCur >= len(m.flatIndex) {
					m.browseCur = len(m.flatIndex) - 1
				}
				if len(m.flatIndex) > 0 {
					m.skipToNextSkill(&m.browseCur, -1)
				}
			case "n", "esc":
				m.confirming = false
			}
			return m, nil
		}

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
		case " ":
			if len(m.flatIndex) > 0 && m.flatIndex[m.browseCur] >= 0 {
				idx := m.flatIndex[m.browseCur]
				m.browsesel[idx] = !m.browsesel[idx]
			}
			return m, nil
		case "d":
			count := 0
			for _, sel := range m.browsesel {
				if sel {
					count++
				}
			}
			if count > 0 {
				m.confirming = true
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

func (m Model) updateCommunity(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.commCur > 0 {
				m.commCur--
			}
		case "down", "j":
			if m.commCur < 3 {
				m.commCur++
			}
		case "enter":
			switch m.commCur {
			case 0: // Browse
				m.view = viewCommunityList
				m.loading = true
				m.commSkills = nil
				m.commMsg = ""
				m.err = nil
				return m, tea.Batch(m.spinner.Tick, m.doBrowseCommunity())
			case 1: // Search
				m.view = viewCommunitySearch
				m.input.SetValue("")
				m.input.Placeholder = "search community skills..."
				m.input.Focus()
				m.commMsg = ""
				m.err = nil
				return m, textinput.Blink
			case 2: // Upload
				m.view = viewUploadSelect
				m.skills = m.store.All()
				m.buildGroups()
				m.writeCur = 0
				m.skipToNextSkill(&m.writeCur, 1)
				m.selected = make(map[int]bool)
				m.err = nil
				m.commMsg = ""
				return m, nil
			case 3: // Back
				m.view = viewMenu
				return m, nil
			}
		}
	}
	return m, nil
}

func (m Model) updateCommunitySearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "enter" {
			query := m.input.Value()
			if query == "" {
				return m, nil
			}
			m.view = viewCommunityList
			m.loading = true
			m.commSkills = nil
			m.commMsg = ""
			m.err = nil
			return m, tea.Batch(m.spinner.Tick, m.doSearchCommunity(query))
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) updateCommunityList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case communityBrowseMsg:
		m.loading = false
		m.err = msg.err
		m.commSkills = msg.skills
		m.commCur = 0
		return m, nil
	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}
		switch msg.String() {
		case "up", "k":
			if m.commCur > 0 {
				m.commCur--
			}
		case "down", "j":
			if m.commCur < len(m.commSkills)-1 {
				m.commCur++
			}
		case "enter":
			if len(m.commSkills) > 0 {
				m.view = viewCommunityDetail
			}
			return m, nil
		case "d":
			// Download selected skill into local library
			if len(m.commSkills) > 0 {
				cs := m.commSkills[m.commCur]
				if m.store.Exists(cs.Name) {
					m.commMsg = "Already in library: " + cs.Name
				} else {
					m.store.Add(cs.Skill)
					m.commMsg = "✓ Downloaded: " + cs.Name
				}
			}
			return m, nil
		}
	}
	return m, nil
}

func (m Model) updateCommunityDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			m.view = viewCommunityList
			return m, nil
		case "d":
			cs := m.commSkills[m.commCur]
			if m.store.Exists(cs.Name) {
				m.commMsg = "Already in library: " + cs.Name
			} else {
				m.store.Add(cs.Skill)
				m.commMsg = "✓ Downloaded: " + cs.Name
			}
			return m, nil
		}
	}
	return m, nil
}

func (m Model) updateUploadSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case communityPublishMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.commMsg = "✓ Published to community!"
		}
		return m, nil
	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}
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
		case "enter":
			if len(m.flatIndex) > 0 && m.flatIndex[m.writeCur] >= 0 {
				sk := m.skills[m.flatIndex[m.writeCur]]
				m.loading = true
				m.commMsg = ""
				m.err = nil
				return m, tea.Batch(m.spinner.Tick, m.doPublish(sk))
			}
		}
	}
	return m, nil
}

func (m Model) doBrowseCommunity() tea.Cmd {
	return func() tea.Msg {
		skills, err := m.hub.Browse()
		return communityBrowseMsg{skills: skills, err: err}
	}
}

func (m Model) doSearchCommunity(query string) tea.Cmd {
	return func() tea.Msg {
		skills, err := m.hub.Search(query)
		return communityBrowseMsg{skills: skills, err: err}
	}
}

func (m Model) doPublish(sk skill.Skill) tea.Cmd {
	return func() tea.Msg {
		err := m.hub.Publish(sk, "anonymous")
		return communityPublishMsg{err: err}
	}
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("⚡ mainstacks") + dimStyle.Render(fmt.Sprintf(" (%d skills in library)", m.store.Count())) + "\n\n")

	switch m.view {
	case viewMenu:
		items := []string{"Ingest repo", "Write skills", "Browse skills", "Community Market", "Quit"}
		for i, item := range items {
			if i == m.cursor {
				b.WriteString(selectedStyle.Render("→ "+item) + "\n")
			} else {
				b.WriteString(itemStyle.Render("  "+item) + "\n")
			}
		}
		b.WriteString("\n" + dimStyle.Render("j/k to move, enter to select, q to quit"))

	case viewIngestInput:
		b.WriteString("  Hit enter to ingest current directory, or type a path:\n\n")
		b.WriteString("  " + m.input.View() + "\n")
		b.WriteString("\n" + dimStyle.Render("esc to go back"))

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
			b.WriteString(m.renderBrowseList())
			if m.confirming {
				count := 0
				for _, sel := range m.browsesel {
					if sel {
						count++
					}
				}
				b.WriteString("\n" + errorStyle.Render(fmt.Sprintf("  Delete %d skill(s)? (y/n)", count)))
			} else {
				b.WriteString("\n" + dimStyle.Render("j/k move, space select, enter view, d delete, esc back"))
			}
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

	case viewCommunity:
		b.WriteString(headerStyle.Render("  Community Market") + "\n\n")
		items := []string{"Browse & download skills", "Search skills", "Upload a skill", "Back"}
		for i, item := range items {
			if i == m.commCur {
				b.WriteString(selectedStyle.Render("  → "+item) + "\n")
			} else {
				b.WriteString(itemStyle.Render("    "+item) + "\n")
			}
		}
		b.WriteString("\n" + dimStyle.Render("j/k to move, enter to select, esc to go back"))

	case viewCommunitySearch:
		b.WriteString(headerStyle.Render("  Search Community") + "\n\n")
		b.WriteString("  " + m.input.View() + "\n")
		b.WriteString("\n" + dimStyle.Render("enter to search, esc to go back"))

	case viewCommunityList:
		b.WriteString(headerStyle.Render("  Community Skills") + "\n\n")
		if m.loading {
			b.WriteString("  " + m.spinner.View() + " Loading community skills...\n")
		} else if m.err != nil {
			b.WriteString("  " + errorStyle.Render("Error: "+m.err.Error()) + "\n")
		} else if len(m.commSkills) == 0 {
			b.WriteString("  No skills published yet. Be the first!\n")
		} else {
			for i, cs := range m.commSkills {
				prefix := "  "
				if i == m.commCur {
					prefix = selectedStyle.Render("→ ")
				}
				author := ""
				if cs.Author != "" {
					author = dimStyle.Render(" by "+cs.Author)
				}
				b.WriteString(fmt.Sprintf("  %s%s [%s]%s\n", prefix, cs.Name, string(cs.Type), author))
			}
		}
		if m.commMsg != "" {
			b.WriteString("\n  " + successStyle.Render(m.commMsg) + "\n")
		}
		b.WriteString("\n" + dimStyle.Render("j/k move, enter view, d download, esc back"))

	case viewCommunityDetail:
		if len(m.commSkills) > 0 {
			cs := m.commSkills[m.commCur]
			b.WriteString(headerStyle.Render("  "+cs.Name) + "\n\n")
			b.WriteString(fmt.Sprintf("  Type:    %s\n", string(cs.Type)))
			b.WriteString(fmt.Sprintf("  Source:  %s\n", cs.Source))
			if cs.Author != "" {
				b.WriteString(fmt.Sprintf("  Author:  %s\n", cs.Author))
			}
			if len(cs.Tags) > 0 {
				b.WriteString(fmt.Sprintf("  Tags:    %s\n", tagStyle.Render(strings.Join(cs.Tags, ", "))))
			}
			b.WriteString("\n")
			b.WriteString(wrapIndent(cs.Summary, "  ", 72))
			if cs.Pattern != "" {
				b.WriteString("\n\n" + dimStyle.Render("  Pattern:") + "\n")
				b.WriteString(wrapIndent(cs.Pattern, "    ", 72))
			}
			if m.commMsg != "" {
				b.WriteString("\n\n  " + successStyle.Render(m.commMsg))
			}
		}
		b.WriteString("\n\n" + dimStyle.Render("d to download, esc to go back"))

	case viewUploadSelect:
		b.WriteString(headerStyle.Render("  Upload to Community") + "\n\n")
		if m.loading {
			b.WriteString("  " + m.spinner.View() + " Publishing...\n")
		} else if m.err != nil {
			b.WriteString("  " + errorStyle.Render(m.err.Error()) + "\n")
		} else if m.commMsg != "" {
			b.WriteString("  " + successStyle.Render(m.commMsg) + "\n")
		} else if len(m.skills) == 0 {
			b.WriteString("  No skills in library. Ingest a repo first.\n")
		} else {
			b.WriteString("  Select a skill to publish:\n\n")
			b.WriteString(m.renderGroupedList(m.writeCur, false))
		}
		b.WriteString("\n" + dimStyle.Render("j/k move, enter to publish, esc back"))
	}

	return b.String()
}

func (m Model) renderBrowseList() string {
	var b strings.Builder
	pos := 0
	for _, grp := range m.groups {
		b.WriteString(groupStyle.Render(fmt.Sprintf("%s (%d)", grp.name, len(grp.skills))) + "\n")
		pos++
		for _, idx := range grp.skills {
			sk := m.skills[idx]
			check := "○"
			if m.browsesel[idx] {
				check = successStyle.Render("●")
			}
			name := fmt.Sprintf("%s %s", check, sk.Name)
			if pos == m.browseCur {
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
				if pos == cursor {
					prefix = successStyle.Render("●") + " "
				} else {
					prefix = "○ "
				}
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

func (m Model) doIngestPath(path string) tea.Cmd {
	return func() tea.Msg {
		skills, err := agent.IngestRepo(context.Background(), m.client, path)
		if err != nil {
			return ingestDoneMsg{err: err}
		}

		for _, sk := range skills {
			if !m.store.Exists(sk.Name) && !m.store.ExistsBySource(sk.Source) {
				m.store.Add(sk)
			}
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
