package raid

import (
	"fmt"
	"strconv"
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

func meterBar(percent float64, width int) string {
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
	if filled > width {
		filled = width
	}

	var barStyle lipgloss.Style
	switch {
	case percent >= 90:
		barStyle = dangerStyle
	case percent >= 70:
		barStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F0C040"))
	default:
		barStyle = positiveStyle
	}

	return barStyle.Render(strings.Repeat("█", filled)) + mutedStyle.Render(strings.Repeat("░", width-filled))
}

func statusCard(title, value, detail string, percent float64, width int) string {
	innerWidth := maxInt(20, width-8)
	var valueStyle lipgloss.Style
	switch {
	case percent >= 90:
		valueStyle = lipgloss.NewStyle().Bold(true).Foreground(colorDanger)
	case percent >= 70:
		valueStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F0C040"))
	default:
		valueStyle = lipgloss.NewStyle().Bold(true).Foreground(colorGreen)
	}

	body := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#C0D0E0")).Render(strings.ToUpper(title))
	body += "\n"
	body += valueStyle.Render(value)
	body += "\n"
	body += meterBar(percent, maxInt(10, innerWidth-2))
	body += "\n"
	body += mutedStyle.Render(detail)
	return panelStyle.Width(innerWidth).Render(body)
}

func statusDiagnosis(snapshot statusSnapshot) string {
	memory := ratio(snapshot.MemoryUsed, snapshot.MemoryTotal)
	disk := ratio(snapshot.DiskUsed, snapshot.DiskTotal)

	pill := func(text string, bg lipgloss.Color) string {
		return lipgloss.NewStyle().Bold(true).Padding(0, 2).Background(bg).Foreground(lipgloss.Color("#FFFFFF")).Render(text)
	}

	switch {
	case snapshot.TemperatureC >= 90:
		return pill(" THERMAL WARNING "+strconv.Itoa(int(snapshot.TemperatureC))+"°C ", colorDanger)
	case disk >= 95:
		return pill(" DISK CRITICAL ", colorDanger)
	case memory >= 95:
		return pill(" MEMORY CRITICAL ", colorDanger)
	case snapshot.CPUPercent >= 95:
		return pill(" CPU PRESSURE ", colorDanger)
	case disk >= 80:
		return pill(" DISK LOW ", lipgloss.Color("#F0C040"))
	case memory >= 80:
		return pill(" MEMORY LOW ", lipgloss.Color("#F0C040"))
	default:
		return lipgloss.NewStyle().Bold(true).Padding(0, 2).Background(lipgloss.Color("#1A3A1A")).Foreground(colorGreen).Render(" SYSTEM NOMINAL ")
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

	header := lipgloss.NewStyle().Bold(true).Foreground(colorLime).Background(lipgloss.Color("#1A2A10")).Padding(0, 1).Render(" RAID // LIVE STATUS ")
	hostInfo := "  " + mutedStyle.Render(m.snapshot.Hostname+"  Linux "+m.snapshot.Kernel)
	diagnosis := statusDiagnosis(m.snapshot)

	memoryPercent := ratio(m.snapshot.MemoryUsed, m.snapshot.MemoryTotal)
	diskPercent := ratio(m.snapshot.DiskUsed, m.snapshot.DiskTotal)
	cardWidth := minInt(42, maxInt(28, (m.width-8)/2))

	cpu := statusCard("CPU", fmt.Sprintf("%5.1f%%", m.snapshot.CPUPercent), fmt.Sprintf("%d cores  load %.2f", m.snapshot.CPUCores, m.snapshot.Load1), m.snapshot.CPUPercent, cardWidth)
	memory := statusCard("Memory", fmt.Sprintf("%5.1f%%", memoryPercent), fmt.Sprintf("%s / %s", formatBytes(int64(m.snapshot.MemoryUsed)), formatBytes(int64(m.snapshot.MemoryTotal))), memoryPercent, cardWidth)
	disk := statusCard("Disk", fmt.Sprintf("%5.1f%%", diskPercent), fmt.Sprintf("%s / %s", formatBytes(int64(m.snapshot.DiskUsed)), formatBytes(int64(m.snapshot.DiskTotal))), diskPercent, cardWidth)

	netThermLabel := "Network"
	netThermValue := fmt.Sprintf("RX %s", formatBytes(int64(m.snapshot.NetworkReceived)))
	netThermDetail := fmt.Sprintf("TX %s", formatBytes(int64(m.snapshot.NetworkSent)))
	netThermPct := 0.0
	if m.snapshot.TemperatureC > 0 {
		netThermLabel = "Network / Thermal"
		netThermValue = fmt.Sprintf("%.1f °C", m.snapshot.TemperatureC)
		netThermDetail = fmt.Sprintf("RX %s  TX %s", formatBytes(int64(m.snapshot.NetworkReceived)), formatBytes(int64(m.snapshot.NetworkSent)))
		netThermPct = m.snapshot.TemperatureC
	}
	network := statusCard(netThermLabel, netThermValue, netThermDetail, netThermPct, cardWidth)

	var cards string
	if m.width >= 78 {
		rowOne := lipgloss.JoinHorizontal(lipgloss.Top, cpu, "  ", memory)
		rowTwo := lipgloss.JoinHorizontal(lipgloss.Top, disk, "  ", network)
		cards = lipgloss.JoinVertical(lipgloss.Left, rowOne, rowTwo)
	} else {
		cards = lipgloss.JoinVertical(lipgloss.Left, cpu, memory, disk, network)
	}

	uptime := (time.Duration(m.snapshot.UptimeSeconds) * time.Second).Round(time.Minute).String()
	footer := mutedStyle.Render(fmt.Sprintf("Uptime %s", uptime))
	if m.snapshot.BatteryPercent > 0 {
		footer = mutedStyle.Render(fmt.Sprintf("Battery %d%% %s", m.snapshot.BatteryPercent, m.snapshot.BatteryStatus)) + "  " + footer
	}
	if m.snapshot.GPU != "" {
		footer = mutedStyle.Render("GPU: "+m.snapshot.GPU) + "  " + footer
	}
	footer += "  " + mutedStyle.Render("r refresh  Esc/q back")

	content := lipgloss.JoinVertical(lipgloss.Left, header+hostInfo, "", diagnosis, "", cards, "", footer)
	return lipgloss.NewStyle().Padding(1, 2).Render(content)
}

func (a *app) runStatusTUI() error {
	_, err := tea.NewProgram(statusModel{loading: true}, tea.WithAltScreen()).Run()
	return err
}
