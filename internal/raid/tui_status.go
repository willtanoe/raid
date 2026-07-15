package raid

import (
	"fmt"
	"sort"
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

func progressBar(percent float64, width int) string {
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

func scoreStyle(score int) lipgloss.Style {
	switch {
	case score >= 80:
		return lipgloss.NewStyle().Bold(true).Foreground(colorGreen)
	case score >= 50:
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F0C040"))
	default:
		return dangerStyle
	}
}

func renderCPUCard(s statusSnapshot, width int) string {
	icon := "◉ CPU"
	titleText := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#BD93F9")).Render(icon)
	headerLen := maxInt(width-lipgloss.Width(icon)-2, 0)
	titleText += "  " + mutedStyle.Render(strings.Repeat("╌", headerLen))

	var lines []string
	lines = append(lines, titleText)

	bar := progressBar(s.CPUPercent, 16)
	pctColor := lipgloss.NewStyle().Foreground(colorGreen)
	if s.CPUPercent > 70 {
		pctColor = lipgloss.NewStyle().Foreground(lipgloss.Color("#F0C040"))
	}
	if s.CPUPercent > 90 {
		pctColor = dangerStyle
	}
	pctText := pctColor.Render(fmt.Sprintf("%5.1f%%", s.CPUPercent))
	tempText := ""
	if s.TemperatureC > 0 {
		tempStyle := lipgloss.NewStyle().Foreground(colorGreen)
		if s.TemperatureC >= 80 {
			tempStyle = dangerStyle
		} else if s.TemperatureC >= 60 {
			tempStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F0C040"))
		}
		tempText = fmt.Sprintf(" @ %s°C", tempStyle.Render(fmt.Sprintf("%.1f", s.TemperatureC)))
	}
	lines = append(lines, fmt.Sprintf("Total  %s  %s%s", bar, pctText, tempText))

	if len(s.PerCoreCPU) > 0 {
		type corePct struct {
			idx int
			val float64
		}
		var cores []corePct
		for i, v := range s.PerCoreCPU {
			if v >= 0 {
				cores = append(cores, corePct{i, v})
			}
		}
		sort.Slice(cores, func(i, j int) bool { return cores[i].val > cores[j].val })
		show := minInt(len(cores), 2)
		for i := 0; i < show; i++ {
			c := cores[i]
			cb := progressBar(c.val, 16)
			lines = append(lines, fmt.Sprintf("Core%-2d %s  %5.1f%%", c.idx+1, cb, c.val))
		}
	}

	loadColor := mutedStyle
	if s.Load1 > float64(s.CPUCores) {
		loadColor = dangerStyle
	} else if s.Load1 > float64(s.CPUCores)*0.8 {
		loadColor = lipgloss.NewStyle().Foreground(lipgloss.Color("#F0C040"))
	}
	lines = append(lines, loadColor.Render(fmt.Sprintf("Load   %.2f, %d cores", s.Load1, s.CPUCores)))

	w := maxInt(28, width-4)
	return panelStyle.Width(w).Render(strings.Join(lines, "\n"))
}

func renderMemoryCard(s statusSnapshot, width int) string {
	icon := "◫ Memory"
	titleText := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#BD93F9")).Render(icon)
	headerLen := maxInt(width-lipgloss.Width(icon)-2, 0)
	titleText += "  " + mutedStyle.Render(strings.Repeat("╌", headerLen))

	var lines []string
	lines = append(lines, titleText)

	memPct := ratio(s.MemoryUsed, s.MemoryTotal)
	bar := progressBar(memPct, 16)
	lines = append(lines, fmt.Sprintf("Used   %s  %5.1f%%", bar, memPct))
	lines = append(lines, fmt.Sprintf("Total  %s / %s", formatBytes(int64(s.MemoryUsed)), formatBytes(int64(s.MemoryTotal))))

	if s.SwapTotal > 0 {
		swapPct := ratio(s.SwapUsed, s.SwapTotal)
		swapBar := progressBar(swapPct, 16)
		swapLine := fmt.Sprintf("Swap   %s  %5.1f%%  %s/%s",
			swapBar, swapPct, formatBytes(int64(s.SwapUsed)), formatBytes(int64(s.SwapTotal)))
		lines = append(lines, swapLine)
	}

	avail := s.MemoryTotal - s.MemoryUsed
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("Avail  %s", formatBytes(int64(avail)))))

	w := maxInt(28, width-4)
	return panelStyle.Width(w).Render(strings.Join(lines, "\n"))
}

func renderDiskCard(s statusSnapshot, width int) string {
	icon := "▥ Disk"
	titleText := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#BD93F9")).Render(icon)
	headerLen := maxInt(width-lipgloss.Width(icon)-2, 0)
	titleText += "  " + mutedStyle.Render(strings.Repeat("╌", headerLen))

	var lines []string
	lines = append(lines, titleText)

	diskPct := ratio(s.DiskUsed, s.DiskTotal)
	bar := progressBar(diskPct, 16)
	lines = append(lines, fmt.Sprintf("Used   %s  %5.1f%%", bar, diskPct))
	lines = append(lines, fmt.Sprintf("Total  %s / %s", formatBytes(int64(s.DiskUsed)), formatBytes(int64(s.DiskTotal))))

	if s.DiskReadRate > 0 || s.DiskWriteRate > 0 {
		ioLine := fmt.Sprintf("I/O    R %.1f  W %.1f MB/s", s.DiskReadRate, s.DiskWriteRate)
		lines = append(lines, mutedStyle.Render(ioLine))
	}

	free := s.DiskTotal - s.DiskUsed
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("Free   %s", formatBytes(int64(free)))))

	w := maxInt(28, width-4)
	return panelStyle.Width(w).Render(strings.Join(lines, "\n"))
}

func renderNetworkCard(s statusSnapshot, width int) string {
	icon := "⇅ Network"
	titleText := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#BD93F9")).Render(icon)
	headerLen := maxInt(width-lipgloss.Width(icon)-2, 0)
	titleText += "  " + mutedStyle.Render(strings.Repeat("╌", headerLen))

	var lines []string
	lines = append(lines, titleText)

	lines = append(lines, fmt.Sprintf("Down   %s", formatBytes(int64(s.NetworkReceived))))
	lines = append(lines, fmt.Sprintf("Up     %s", formatBytes(int64(s.NetworkSent))))

	if s.TemperatureC > 0 {
		tempColor := lipgloss.NewStyle().Foreground(colorGreen)
		if s.TemperatureC >= 80 {
			tempColor = dangerStyle
		} else if s.TemperatureC >= 60 {
			tempColor = lipgloss.NewStyle().Foreground(lipgloss.Color("#F0C040"))
		}
		lines = append(lines, tempColor.Render(fmt.Sprintf("Thermal %.1f °C", s.TemperatureC)))
	}

	w := maxInt(28, width-4)
	return panelStyle.Width(w).Render(strings.Join(lines, "\n"))
}

func renderProcessCard(s statusSnapshot, width int) string {
	if len(s.TopProcesses) == 0 {
		return ""
	}
	icon := "❊ Processes"
	titleText := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#BD93F9")).Render(icon)
	headerLen := maxInt(width-lipgloss.Width(icon)-2, 0)
	titleText += "  " + mutedStyle.Render(strings.Repeat("╌", headerLen))

	var lines []string
	lines = append(lines, titleText)

	for i, p := range s.TopProcesses[:minInt(3, len(s.TopProcesses))] {
		bar := progressBar(p.CPUPercent, 12)
		name := p.Name
		if len(name) > 14 {
			name = name[:13] + "…"
		}
		memText := ""
		if p.MemoryBytes > 0 {
			memText = "  " + formatBytes(int64(p.MemoryBytes))
		}
		lines = append(lines, fmt.Sprintf("#%-5d %s  %5.1f%%  %s%s", i+1, bar, p.CPUPercent, name, memText))
	}

	w := maxInt(28, width-4)
	return panelStyle.Width(w).Render(strings.Join(lines, "\n"))
}

func renderBatteryCard(s statusSnapshot, width int) string {
	if s.BatteryPercent <= 0 {
		return ""
	}
	icon := "◪ Battery"
	titleText := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#BD93F9")).Render(icon)
	headerLen := maxInt(width-lipgloss.Width(icon)-2, 0)
	titleText += "  " + mutedStyle.Render(strings.Repeat("╌", headerLen))

	var lines []string
	lines = append(lines, titleText)

	pct := float64(s.BatteryPercent)
	bar := progressBar(pct, 16)
	batteryColor := lipgloss.NewStyle().Foreground(colorGreen)
	if pct < 20 {
		batteryColor = dangerStyle
	} else if pct < 50 {
		batteryColor = lipgloss.NewStyle().Foreground(lipgloss.Color("#F0C040"))
	}
	lines = append(lines, fmt.Sprintf("Level  %s  %s", bar, batteryColor.Render(fmt.Sprintf("%5.1f%%", pct))))
	lines = append(lines, fmt.Sprintf("Status %s", s.BatteryStatus))

	w := maxInt(28, width-4)
	return panelStyle.Width(w).Render(strings.Join(lines, "\n"))
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
	hostInfo := mutedStyle.Render("  " + m.snapshot.Hostname + "  Linux " + m.snapshot.Kernel)
	scoreText := scoreStyle(m.snapshot.HealthScore).Render(fmt.Sprintf(" ● %d", m.snapshot.HealthScore))
	healthLabel := mutedStyle.Render("  Health") + scoreText
	topLine := lipgloss.JoinHorizontal(lipgloss.Left, header, hostInfo, healthLabel)

	cardWidth := maxInt(28, minInt(48, (m.width-8)/2))

	var cards []string
	cards = append(cards, renderCPUCard(m.snapshot, cardWidth))
	cards = append(cards, renderMemoryCard(m.snapshot, cardWidth))
	cards = append(cards, renderDiskCard(m.snapshot, cardWidth))
	cards = append(cards, renderNetworkCard(m.snapshot, cardWidth))
	if procCard := renderProcessCard(m.snapshot, cardWidth); procCard != "" {
		cards = append(cards, procCard)
	}
	if batCard := renderBatteryCard(m.snapshot, cardWidth); batCard != "" {
		cards = append(cards, batCard)
	}

	var cardLayout string
	if m.width >= 78 {
		for i := 0; i < len(cards); i += 2 {
			left := cards[i]
			right := ""
			if i+1 < len(cards) {
				right = cards[i+1]
			}
			row := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
			if i > 0 {
				cardLayout += "\n"
			}
			cardLayout += row
		}
	} else {
		for i, c := range cards {
			if i > 0 {
				cardLayout += "\n"
			}
			cardLayout += c
		}
	}

	uptime := (time.Duration(m.snapshot.UptimeSeconds) * time.Second).Round(time.Minute).String()
	footer := mutedStyle.Render(fmt.Sprintf("Uptime %s", uptime))
	if m.snapshot.GPU != "" {
		footer = mutedStyle.Render("GPU: "+m.snapshot.GPU) + "  " + footer
	}
	footer += "  " + mutedStyle.Render("r refresh  Esc/q back")

	content := lipgloss.JoinVertical(lipgloss.Left, topLine, "", cardLayout, "", footer)
	return lipgloss.NewStyle().Padding(1, 2).Render(content)
}

func (a *app) runStatusTUI() error {
	_, err := tea.NewProgram(statusModel{loading: true}, tea.WithAltScreen()).Run()
	return err
}
