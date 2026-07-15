package raid

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type statusResultMsg struct {
	snapshot statusSnapshot
	err      error
}

type statusTickMsg time.Time

type statusModel struct {
	width    int
	height   int
	loading  bool
	snapshot statusSnapshot
	err      error
}

func collectStatusCmd() tea.Cmd {
	return func() tea.Msg {
		snapshot, err := collectStatus()
		return statusResultMsg{snapshot: snapshot, err: err}
	}
}

func statusTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(now time.Time) tea.Msg { return statusTickMsg(now) })
}

func (m statusModel) Init() tea.Cmd { return collectStatusCmd() }

func (m statusModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "r":
			if !m.loading {
				m.loading = true
				return m, collectStatusCmd()
			}
		}
	case statusResultMsg:
		m.loading = false
		m.snapshot = msg.snapshot
		m.err = msg.err
		return m, statusTickCmd()
	case statusTickMsg:
		if !m.loading {
			m.loading = true
			return m, collectStatusCmd()
		}
	}
	return m, nil
}

func ratio(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(used) / float64(total)
}

func meter(percent float64, width int) string {
	if width < 8 {
		width = 8
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	filled := int(percent * float64(width) / 100)
	return positiveStyle.Render(strings.Repeat("#", filled)) + mutedStyle.Render(strings.Repeat("-", width-filled))
}

func statusCard(title, value, detail string, percent float64, width int) string {
	innerWidth := maxInt(18, width-6)
	body := titleStyle.Render(strings.ToUpper(title)) + "\n\n"
	body += lipgloss.NewStyle().Bold(true).Render(value) + "\n"
	body += meter(percent, maxInt(10, innerWidth-2)) + "\n"
	body += mutedStyle.Render(detail)
	return panelStyle.Width(innerWidth).Render(body)
}

func statusDiagnosis(snapshot statusSnapshot) string {
	memory := ratio(snapshot.MemoryUsed, snapshot.MemoryTotal)
	disk := ratio(snapshot.DiskUsed, snapshot.DiskTotal)
	switch {
	case snapshot.TemperatureC >= 90:
		return dangerStyle.Render("THERMAL PRESSURE") + "  system temperature is critically high"
	case disk >= 90:
		return dangerStyle.Render("DISK PRESSURE") + "  free storage is running low"
	case memory >= 90:
		return dangerStyle.Render("MEMORY PRESSURE") + "  available memory is running low"
	case snapshot.CPUPercent >= 90:
		return dangerStyle.Render("CPU PRESSURE") + "  sustained load may affect responsiveness"
	default:
		return positiveStyle.Render("SYSTEM NOMINAL") + "  no immediate pressure detected"
	}
}

func (m statusModel) View() string {
	if m.width == 0 {
		return ""
	}
	if m.err != nil {
		return panelStyle.Render(dangerStyle.Render("Status collection failed") + "\n" + m.err.Error() + "\n\nEsc/q back  r retry")
	}
	if m.snapshot.Timestamp == "" {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, titleStyle.Render("RAID // COLLECTING SYSTEM SIGNALS..."))
	}

	header := titleStyle.Render("RAID // LIVE STATUS") + "  " + mutedStyle.Render(m.snapshot.Hostname+"  Linux "+m.snapshot.Kernel)
	diagnosis := statusDiagnosis(m.snapshot)
	memoryPercent := ratio(m.snapshot.MemoryUsed, m.snapshot.MemoryTotal)
	diskPercent := ratio(m.snapshot.DiskUsed, m.snapshot.DiskTotal)
	cardWidth := minInt(39, maxInt(27, (m.width-7)/2))
	cpu := statusCard("CPU", fmt.Sprintf("%5.1f%%", m.snapshot.CPUPercent), fmt.Sprintf("%d cores  load %.2f", m.snapshot.CPUCores, m.snapshot.Load1), m.snapshot.CPUPercent, cardWidth)
	memory := statusCard("Memory", fmt.Sprintf("%5.1f%%", memoryPercent), fmt.Sprintf("%s / %s", formatBytes(int64(m.snapshot.MemoryUsed)), formatBytes(int64(m.snapshot.MemoryTotal))), memoryPercent, cardWidth)
	disk := statusCard("Disk", fmt.Sprintf("%5.1f%%", diskPercent), fmt.Sprintf("%s / %s", formatBytes(int64(m.snapshot.DiskUsed)), formatBytes(int64(m.snapshot.DiskTotal))), diskPercent, cardWidth)
	networkDetail := fmt.Sprintf("RX %s  TX %s", formatBytes(int64(m.snapshot.NetworkReceived)), formatBytes(int64(m.snapshot.NetworkSent)))
	temperature := "sensor unavailable"
	thermalPercent := 0.0
	if m.snapshot.TemperatureC > 0 {
		temperature = fmt.Sprintf("%.1f C", m.snapshot.TemperatureC)
		thermalPercent = m.snapshot.TemperatureC
	}
	network := statusCard("Network / Thermal", temperature, networkDetail, thermalPercent, cardWidth)

	var cards string
	if m.width >= 78 {
		rowOne := lipgloss.JoinHorizontal(lipgloss.Top, cpu, "  ", memory)
		rowTwo := lipgloss.JoinHorizontal(lipgloss.Top, disk, "  ", network)
		cards = lipgloss.JoinVertical(lipgloss.Left, rowOne, rowTwo)
	} else {
		cards = lipgloss.JoinVertical(lipgloss.Left, cpu, memory, disk, network)
	}
	footer := mutedStyle.Render(fmt.Sprintf("Updated %s  Uptime %s", m.snapshot.Timestamp, (time.Duration(m.snapshot.UptimeSeconds) * time.Second).Round(time.Minute)))
	if m.snapshot.BatteryPercent > 0 {
		footer = mutedStyle.Render(fmt.Sprintf("Battery %d%% %s", m.snapshot.BatteryPercent, m.snapshot.BatteryStatus)) + "  " + footer
	}
	if m.snapshot.GPU != "" {
		footer = mutedStyle.Render("GPU: "+m.snapshot.GPU) + "  " + footer
	}
	footer += "  " + mutedStyle.Render("Esc/q back  r refresh")
	content := lipgloss.JoinVertical(lipgloss.Left, header, diagnosis, "", cards, "", footer)
	return lipgloss.NewStyle().Padding(1, 2).Render(content)
}

func (a *app) runStatusTUI() error {
	_, err := tea.NewProgram(statusModel{loading: true}, tea.WithAltScreen()).Run()
	return err
}
