package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cjhargre/hopesandmemes/internal/agent"
	"github.com/cjhargre/hopesandmemes/internal/gemini"
	"github.com/cjhargre/hopesandmemes/internal/router"
	"github.com/cjhargre/hopesandmemes/internal/skill"
)

type view int

const (
	viewMenu view = iota
	viewIngest
	viewQuery
	viewBrowse
)

type Model struct {
	view      view
	cursor    int
	input     textinput.Model
	spinner   spinner.Model
	client    *gemini.Client
	store     *skill.SQLiteStore
	router    *router.Router
	results   []string
	err       error
	loading   bool
	skills    []skill.Skill
}

type ingestDoneMsg struct {
	results []string
	err     error
}

type queryDoneMsg struct {
	answer string
	err    error
}

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	itemStyle    = lipgloss.NewStyle().PaddingLeft(2)
	selectedStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func New(client *gemini.Client, store *skill.SQLiteStore) Model {
	ti := textinput.New()
	ti.Placeholder = "enter path or query..."
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	r := router.New(client, store)

	return Model{
		view:    viewMenu,
		input:   ti,
		spinner: sp,
		client:  client,
		store:   store,
		router:  r,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.view == viewMenu {
				return m, tea.Quit
			}
			m.view = viewMenu
			m.results = nil
			m.err = nil
			m.loading = false
			return m, nil
		case "esc":
			m.view = viewMenu
			m.results = nil
			m.err = nil
			m.loading = false
			return m, nil
		}
	}

	switch m.view {
	case viewMenu:
		return m.updateMenu(msg)
	case viewIngest:
		return m.updateIngest(msg)
	case viewQuery:
		return m.updateQuery(msg)
	case viewBrowse:
		return m.updateBrowse(msg)
	}
	return m, nil
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
				m.view = viewIngest
				m.input.SetValue("")
				m.input.Placeholder = "path to file or folder..."
				m.results = nil
				return m, textinput.Blink
			case 1:
				m.view = viewQuery
				m.input.SetValue("")
				m.input.Placeholder = "ask a question..."
				m.results = nil
				return m, textinput.Blink
			case 2:
				m.view = viewBrowse
				m.skills = m.store.All()
				return m, nil
			case 3:
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m Model) updateIngest(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "enter" && !m.loading {
			path := m.input.Value()
			if path == "" {
				return m, nil
			}
			m.loading = true
			m.results = nil
			return m, tea.Batch(m.spinner.Tick, m.doIngest(path))
		}
	case ingestDoneMsg:
		m.loading = false
		m.results = msg.results
		m.err = msg.err
		return m, nil
	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	if !m.loading {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updateQuery(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "enter" && !m.loading {
			q := m.input.Value()
			if q == "" {
				return m, nil
			}
			m.loading = true
			m.results = nil
			return m, tea.Batch(m.spinner.Tick, m.doQuery(q))
		}
	case queryDoneMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.results = []string{msg.answer}
		}
		return m, nil
	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	if !m.loading {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updateBrowse(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("⚡ hopesandmemes") + "\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("  %d skills ingested", m.store.Count())) + "\n\n")

	switch m.view {
	case viewMenu:
		items := []string{"Ingest files", "Query skills", "Browse skills", "Quit"}
		for i, item := range items {
			if i == m.cursor {
				b.WriteString(selectedStyle.Render("→ " + item) + "\n")
			} else {
				b.WriteString(itemStyle.Render("  " + item) + "\n")
			}
		}
		b.WriteString("\n" + dimStyle.Render("j/k to move, enter to select, q to quit"))

	case viewIngest:
		b.WriteString("  Ingest path:\n")
		b.WriteString("  " + m.input.View() + "\n\n")
		if m.loading {
			b.WriteString("  " + m.spinner.View() + " ingesting...\n")
		}
		for _, r := range m.results {
			b.WriteString("  " + r + "\n")
		}
		if m.err != nil {
			b.WriteString("  " + errorStyle.Render(m.err.Error()) + "\n")
		}
		b.WriteString("\n" + dimStyle.Render("esc to go back"))

	case viewQuery:
		b.WriteString("  Ask a question:\n")
		b.WriteString("  " + m.input.View() + "\n\n")
		if m.loading {
			b.WriteString("  " + m.spinner.View() + " thinking...\n")
		}
		for _, r := range m.results {
			b.WriteString("  " + r + "\n")
		}
		if m.err != nil {
			b.WriteString("  " + errorStyle.Render(m.err.Error()) + "\n")
		}
		b.WriteString("\n" + dimStyle.Render("esc to go back"))

	case viewBrowse:
		if len(m.skills) == 0 {
			b.WriteString("  No skills ingested yet.\n")
		}
		for _, sk := range m.skills {
			b.WriteString(fmt.Sprintf("  [%s] %s\n", successStyle.Render(string(sk.Type)), sk.Source))
			summary := sk.Summary
			if len(summary) > 100 {
				summary = summary[:100] + "..."
			}
			b.WriteString("    " + dimStyle.Render(summary) + "\n\n")
		}
		b.WriteString(dimStyle.Render("esc to go back"))
	}

	return b.String()
}

func (m Model) doIngest(path string) tea.Cmd {
	return func() tea.Msg {
		files, err := loadFiles(path)
		if err != nil {
			return ingestDoneMsg{err: err}
		}

		var results []string
		classifier := agent.NewClassifier(m.client)

		for _, f := range files {
			ctx := context.Background()
			fileType, err := classifier.Classify(ctx, f)
			if err != nil {
				results = append(results, errorStyle.Render("✗ "+f.Path+": "+err.Error()))
				continue
			}

			a := agent.NewGenericAgent(m.client, fileType)
			sk, err := a.Ingest(ctx, f)
			if err != nil {
				results = append(results, errorStyle.Render("✗ "+f.Path+": "+err.Error()))
				continue
			}

			m.store.Add(sk)
			results = append(results, successStyle.Render(fmt.Sprintf("✓ %s → %s", f.Path, sk.Type)))
		}

		return ingestDoneMsg{results: results}
	}
}

func (m Model) doQuery(question string) tea.Cmd {
	return func() tea.Msg {
		answer, err := m.router.Query(context.Background(), question)
		return queryDoneMsg{answer: answer, err: err}
	}
}

func loadFiles(path string) ([]agent.File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	var files []agent.File
	if !info.IsDir() {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return []agent.File{{Path: path, Content: string(content)}}, nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(path, e.Name()))
		if err != nil {
			continue
		}
		files = append(files, agent.File{Path: e.Name(), Content: string(content)})
	}
	return files, nil
}
