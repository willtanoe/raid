package raid

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type analyzeResultMsg struct {
	path    string
	entries []pathInfo
	err     error
}

type analyzeDeleteMsg struct {
	path string
	err  error
}

type analyzeModel struct {
	app        *app
	width      int
	height     int
	path       string
	entries    []pathInfo
	cursor     int
	offset     int
	loading    bool
	confirming bool
	deleting   bool
	err        error
}

func scanDirectoryCmd(path string) tea.Cmd {
	return func() tea.Msg {
		entries, err := scanDirectory(path)
		return analyzeResultMsg{path: path, entries: entries, err: err}
	}
}

func deleteAnalyzePathCmd(a *app, path string) tea.Cmd {
	return func() tea.Msg {
		quiet := *a
		quiet.out = io.Discard
		quiet.errOut = io.Discard
		err := quiet.removePath(path, "analyze", commonFlags{yes: true})
		return analyzeDeleteMsg{path: path, err: err}
	}
}

func (m analyzeModel) Init() tea.Cmd { return scanDirectoryCmd(m.path) }

func (m analyzeModel) visibleRows() int {
	return maxInt(4, m.height-9)
}

func (m *analyzeModel) normalizeViewport() {
	if len(m.entries) == 0 {
		m.cursor, m.offset = 0, 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.entries) {
		m.cursor = len(m.entries) - 1
	}
	rows := m.visibleRows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
}

func (m analyzeModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.normalizeViewport()
	case analyzeResultMsg:
		if msg.path == m.path {
			m.loading = false
			m.entries = msg.entries
			m.err = msg.err
			m.cursor, m.offset = 0, 0
		}
	case analyzeDeleteMsg:
		m.deleting = false
		m.confirming = false
		m.err = msg.err
		if msg.err == nil {
			m.loading = true
			return m, scanDirectoryCmd(m.path)
		}
	case tea.KeyMsg:
		if m.deleting {
			return m, nil
		}
		if m.confirming {
			switch msg.String() {
			case "y", "Y":
				if len(m.entries) > 0 {
					m.deleting = true
					return m, deleteAnalyzePathCmd(m.app, m.entries[m.cursor].path)
				}
			case "n", "N", "esc":
				m.confirming = false
			}
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "up", "k":
			m.cursor--
			m.normalizeViewport()
		case "down", "j":
			m.cursor++
			m.normalizeViewport()
		case "g", "home":
			m.cursor = 0
			m.normalizeViewport()
		case "G", "end":
			m.cursor = len(m.entries) - 1
			m.normalizeViewport()
		case "enter", "right", "l":
			if len(m.entries) > 0 && m.entries[m.cursor].isDir {
				m.path = m.entries[m.cursor].path
				m.entries = nil
				m.loading = true
				m.err = nil
				return m, scanDirectoryCmd(m.path)
			}
		case "backspace", "left", "h":
			parent := filepath.Dir(m.path)
			if parent != m.path {
				m.path = parent
				m.entries = nil
				m.loading = true
				m.err = nil
				return m, scanDirectoryCmd(m.path)
			}
		case "r":
			m.loading = true
			m.err = nil
			return m, scanDirectoryCmd(m.path)
		case "d":
			if len(m.entries) > 0 {
				m.confirming = true
			}
		}
	}
	return m, nil
}

func truncateMiddle(value string, width int) string {
	if width <= 3 || len(value) <= width {
		return value
	}
	left := (width - 3) / 2
	right := width - 3 - left
	return value[:left] + "..." + value[len(value)-right:]
}

func (m analyzeModel) View() string {
	if m.width == 0 {
		return ""
	}
	var body strings.Builder
	body.WriteString(titleStyle.Render("RAID // DISK ANALYZER"))
	body.WriteString("\n")
	body.WriteString(mutedStyle.Render(truncateMiddle(m.path, maxInt(30, m.width-8))))
	body.WriteString("\n\n")
	if m.loading {
		body.WriteString(positiveStyle.Render("Scanning directory sizes..."))
		body.WriteString("\n")
	}
	if m.err != nil {
		body.WriteString(dangerStyle.Render(m.err.Error()))
		body.WriteString("\n")
	}
	if !m.loading && len(m.entries) == 0 && m.err == nil {
		body.WriteString(mutedStyle.Render("Directory is empty."))
		body.WriteString("\n")
	}
	rows := m.visibleRows()
	end := minInt(len(m.entries), m.offset+rows)
	nameWidth := maxInt(20, m.width-28)
	for index := m.offset; index < end; index++ {
		entry := m.entries[index]
		kind := " "
		if entry.isDir {
			kind = "/"
		}
		name := truncateMiddle(filepath.Base(entry.path), nameWidth)
		line := fmt.Sprintf("%s %-*s %10s", kind, nameWidth, name, formatBytes(entry.size))
		if index == m.cursor {
			body.WriteString(activeStyle.Render("> " + line))
		} else {
			body.WriteString("  " + line)
		}
		body.WriteString("\n")
	}
	if m.confirming && len(m.entries) > 0 {
		body.WriteString("\n")
		body.WriteString(dangerStyle.Render("Move to Trash? "))
		body.WriteString(truncateMiddle(m.entries[m.cursor].path, maxInt(24, m.width-24)))
		body.WriteString("  [y/N]")
	}
	if m.deleting {
		body.WriteString("\n" + dangerStyle.Render("Moving selected entry to Trash..."))
	}
	body.WriteString("\n")
	body.WriteString(mutedStyle.Render("j/k move  Enter open  Backspace parent  d trash  r rescan  Esc/q back"))
	return lipgloss.NewStyle().Padding(1, 2).Render(body.String())
}

func (a *app) runAnalyzeTUI(root string) error {
	model := analyzeModel{app: a, path: filepath.Clean(root), loading: true}
	_, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}
