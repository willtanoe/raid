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
	totalSize  int64
	maxSize    int64
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
	return maxInt(4, m.height-8)
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

func (m *analyzeModel) recomputeStats() {
	m.totalSize = 0
	m.maxSize = 1
	for _, e := range m.entries {
		if e.size > 0 {
			m.totalSize += e.size
		}
		if e.size > m.maxSize {
			m.maxSize = e.size
		}
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
			m.recomputeStats()
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
			case "y", "Y", "enter":
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

func sizeColorForPercent(percent float64) lipgloss.Style {
	switch {
	case percent >= 50:
		return dangerStyle
	case percent >= 20:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F0C040"))
	case percent >= 5:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#56B6C2"))
	default:
		return mutedStyle
	}
}

func (m analyzeModel) View() string {
	if m.width == 0 {
		return ""
	}
	var body strings.Builder

	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#BD93F9")).Render("Analyze Disk")
	body.WriteString(header)
	body.WriteString("\n")
	pathDisplay := mutedStyle.Render(truncateMiddle(m.path, maxInt(30, m.width-8)))
	body.WriteString(pathDisplay)
	if m.totalSize > 0 {
		body.WriteString(mutedStyle.Render("  |  Total: " + formatBytes(m.totalSize)))
	}
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
	nameWidth := maxInt(18, m.width-38)

	for index := m.offset; index < end; index++ {
		entry := m.entries[index]

		icon := "  "
		if entry.isDir {
			icon = "📁"
		}

		name := truncateMiddle(filepath.Base(entry.path), nameWidth)
		size := formatBytes(entry.size)
		percent := 0.0
		if m.maxSize > 0 && entry.size >= 0 {
			percent = float64(entry.size) / float64(m.maxSize) * 100
		}

		barWidth := 10
		filled := int(percent * float64(barWidth) / 100)
		if filled > barWidth {
			filled = barWidth
		}
		bar := sizeColorForPercent(percent).Render(strings.Repeat("█", filled)) + mutedStyle.Render(strings.Repeat("░", barWidth-filled))

		idxLabel := fmt.Sprintf("%3d.", index+1)

		line := fmt.Sprintf("%s %s %s %s %10s", idxLabel, bar, icon, name, size)

		if index == m.cursor {
			cursorLine := activeStyle.Render("> " + line)
			body.WriteString(cursorLine)
		} else {
			body.WriteString("   " + line)
		}
		body.WriteString("\n")
	}

	if m.confirming && len(m.entries) > 0 {
		body.WriteString("\n")
		body.WriteString(dangerStyle.Render("Move to Trash? "))
		body.WriteString(truncateMiddle(m.entries[m.cursor].path, maxInt(24, m.width-24)))
		body.WriteString("  [y/N]")
	} else if m.deleting {
		body.WriteString("\n" + dangerStyle.Render("Moving selected entry to Trash..."))
	}

	body.WriteString("\n")
	footer := mutedStyle.Render("j/k ↑↓ move  Enter open  Backspace parent  d trash  r rescan  Esc/q back")
	body.WriteString(footer)

	return lipgloss.NewStyle().Padding(1, 2).Render(body.String())
}

func (a *app) runAnalyzeTUI(root string) error {
	model := analyzeModel{app: a, path: filepath.Clean(root), loading: true}
	_, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}
