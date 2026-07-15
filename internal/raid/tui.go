package raid

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

var (
	colorLime     = lipgloss.Color("#B7F34A")
	colorGreen    = lipgloss.Color("#78D64B")
	colorMuted    = lipgloss.Color("#7B8496")
	colorPanel    = lipgloss.Color("#252A34")
	colorDanger   = lipgloss.Color("#FF6B6B")
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorLime)
	mutedStyle    = lipgloss.NewStyle().Foreground(colorMuted)
	activeStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#10130D")).Background(colorLime).Padding(0, 1)
	panelStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorPanel).Padding(1, 2)
	dangerStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorDanger)
	positiveStyle = lipgloss.NewStyle().Foreground(colorGreen)
)

func isInteractiveTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

type menuItem struct {
	name        string
	description string
	command     []string
}

var mainMenuItems = []menuItem{
	{name: "Clean", description: "Preview rebuildable user and developer caches", command: []string{"clean"}},
	{name: "Uninstall", description: "Remove an exact APT, DNF, Pacman, Snap, or Flatpak package", command: []string{"uninstall"}},
	{name: "Optimize", description: "Preview bounded Linux maintenance tasks (cross-distro)", command: []string{"optimize"}},
	{name: "Analyze", description: "Explore disk usage and move selected entries to Trash", command: []string{"analyze"}},
	{name: "Status", description: "Watch CPU, memory, disk, network, and thermal health", command: []string{"status"}},
	{name: "Purge", description: "Preview rebuildable artifacts in the current project", command: []string{"purge"}},
	{name: "Installer", description: "Review installer files older than seven days", command: []string{"installer"}},
	{name: "Update", description: "Check system updates (APT, DNF, Pacman, Snap, Flatpak)", command: []string{"update"}},
	{name: "Docker", description: "Preview and clean unused Docker containers and images", command: []string{"docker"}},
	{name: "Search", description: "Find large or old files by size, age, or pattern", command: []string{"search"}},
	{name: "Convert", description: "Convert shell history between zsh, fish, and bash formats", command: []string{"convert"}},
	{name: "History", description: "Read the local operation audit log", command: []string{"history"}},
}

type mainMenuModel struct {
	width     int
	height    int
	cursor    int
	inputMode bool
	input     string
	selected  []string
}

func (m mainMenuModel) Init() tea.Cmd { return nil }

func (m mainMenuModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		if m.inputMode {
			switch msg.String() {
			case "esc":
				m.inputMode = false
				m.input = ""
			case "enter":
				if strings.TrimSpace(m.input) != "" {
					m.selected = []string{"uninstall", strings.TrimSpace(m.input)}
					return m, tea.Quit
				}
			case "backspace":
				if len(m.input) > 0 {
					m.input = m.input[:len(m.input)-1]
				}
			default:
				if len(msg.Runes) > 0 && len(m.input) < 120 {
					m.input += string(msg.Runes)
				}
			}
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(mainMenuItems)-1 {
				m.cursor++
			}
		case "enter", " ":
			item := mainMenuItems[m.cursor]
			if item.name == "Uninstall" {
				m.inputMode = true
				return m, nil
			}
			m.selected = item.command
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m mainMenuModel) View() string {
	if m.width == 0 {
		return ""
	}
	contentWidth := minInt(72, maxInt(42, m.width-8))
	var body strings.Builder
	body.WriteString(titleStyle.Render("RAID // LINUX TOOLKIT"))
	body.WriteString("\n")
	body.WriteString(mutedStyle.Render("Safe cleanup. Exact plans. No background daemon."))
	body.WriteString("\n\n")
	for index, item := range mainMenuItems {
		label := fmt.Sprintf("%-11s", item.name)
		if index == m.cursor {
			body.WriteString(activeStyle.Render("> " + label))
			body.WriteString("  " + item.description)
		} else {
			body.WriteString(mutedStyle.Render("  " + label))
			body.WriteString("  " + item.description)
		}
		body.WriteString("\n")
	}
	if m.inputMode {
		body.WriteString("\n")
		body.WriteString(titleStyle.Render("Exact package name"))
		body.WriteString("\n")
		body.WriteString("  " + m.input + "_")
		body.WriteString("\n")
		body.WriteString(mutedStyle.Render("Enter confirm  Esc cancel"))
	} else {
		body.WriteString("\n")
		body.WriteString(mutedStyle.Render("Up/Down or j/k move  Enter select  q quit"))
	}
	panel := panelStyle.Width(contentWidth).Render(body.String())
	if m.height > lipgloss.Height(panel) {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
	}
	return panel
}

func (a *app) runMainTUI() ([]string, error) {
	program := tea.NewProgram(mainMenuModel{}, tea.WithAltScreen())
	result, err := program.Run()
	if err != nil {
		return nil, err
	}
	model, ok := result.(mainMenuModel)
	if !ok {
		return nil, nil
	}
	return model.selected, nil
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
