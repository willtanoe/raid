package raid

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func (a *app) runAnalyze(args []string) error {
	jsonOutput := false
	textOutput := false
	var paths []string
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		case "--text":
			textOutput = true
		default:
			if strings.HasPrefix(arg, "-") {
				return fmt.Errorf("unknown option %q", arg)
			}
			paths = append(paths, arg)
		}
	}
	if len(paths) > 1 {
		return errors.New("usage: raid analyze [path] [--json]")
	}
	root := a.home
	var err error
	if len(paths) == 1 {
		root, err = filepath.Abs(paths[0])
		if err != nil {
			return err
		}
	}
	if !jsonOutput && !textOutput && isInteractiveTerminal() {
		return a.runAnalyzeTUI(root)
	}
	results, err := scanDirectory(root)
	if err != nil {
		return err
	}
	return a.renderAnalyze(root, results, jsonOutput)
}

func scanDirectory(root string) ([]pathInfo, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	results := make([]pathInfo, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		size, sizeErr := pathSize(path)
		if sizeErr != nil {
			continue
		}
		info, _ := entry.Info()
		item := pathInfo{path: path, size: size, isDir: entry.IsDir()}
		if info != nil {
			item.modTime = info.ModTime()
		}
		results = append(results, item)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].size > results[j].size })
	return results, nil
}

func (a *app) renderAnalyze(root string, results []pathInfo, jsonOutput bool) error {
	if jsonOutput {
		type outputItem struct {
			Path string `json:"path"`
			Size int64  `json:"size_bytes"`
		}
		output := struct {
			Path    string       `json:"path"`
			Entries []outputItem `json:"entries"`
		}{Path: root}
		for _, item := range results {
			output.Entries = append(output.Entries, outputItem{Path: item.path, Size: item.size})
		}
		encoder := json.NewEncoder(a.out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(output)
	}
	fmt.Fprintf(a.out, "Disk usage: %s\n", root)
	for _, item := range results {
		fmt.Fprintf(a.out, "%10s  %s\n", formatBytes(item.size), item.path)
	}
	return nil
}

type cpuTimes struct {
	idle  uint64
	total uint64
}

type statusSnapshot struct {
	Timestamp       string  `json:"timestamp"`
	Hostname        string  `json:"hostname"`
	Kernel          string  `json:"kernel"`
	CPUPercent      float64 `json:"cpu_percent"`
	CPUCores        int     `json:"cpu_cores"`
	MemoryUsed      uint64  `json:"memory_used_bytes"`
	MemoryTotal     uint64  `json:"memory_total_bytes"`
	DiskUsed        uint64  `json:"disk_used_bytes"`
	DiskTotal       uint64  `json:"disk_total_bytes"`
	Load1           float64 `json:"load_1"`
	UptimeSeconds   float64 `json:"uptime_seconds"`
	TemperatureC    float64 `json:"temperature_c,omitempty"`
	NetworkReceived uint64  `json:"network_received_bytes"`
	NetworkSent     uint64  `json:"network_sent_bytes"`
	BatteryPercent  int     `json:"battery_percent,omitempty"`
	BatteryStatus   string  `json:"battery_status,omitempty"`
	GPU             string  `json:"gpu,omitempty"`
}

func readCPUTimes() (cpuTimes, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return cpuTimes{}, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return cpuTimes{}, errors.New("missing aggregate CPU data")
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuTimes{}, errors.New("invalid /proc/stat CPU data")
	}
	var values []uint64
	for _, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return cpuTimes{}, err
		}
		values = append(values, value)
	}
	var total uint64
	for _, value := range values {
		total += value
	}
	idle := values[3]
	if len(values) > 4 {
		idle += values[4]
	}
	return cpuTimes{idle: idle, total: total}, nil
}

func readKeyValues(path string) map[string]uint64 {
	result := make(map[string]uint64)
	file, err := os.Open(path)
	if err != nil {
		return result
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(strings.ReplaceAll(scanner.Text(), ":", ""))
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err == nil {
			result[fields[0]] = value * 1024
		}
	}
	return result
}

func collectNetwork() (uint64, uint64) {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	defer file.Close()
	var received, sent uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if strings.TrimSpace(parts[0]) == "lo" {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) >= 9 {
			rx, _ := strconv.ParseUint(fields[0], 10, 64)
			tx, _ := strconv.ParseUint(fields[8], 10, 64)
			received += rx
			sent += tx
		}
	}
	return received, sent
}

func firstTemperature() float64 {
	paths, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
		if err == nil && value > 0 {
			if value > 1000 {
				value /= 1000
			}
			return value
		}
	}
	return 0
}

func readBattery() (int, string) {
	entries, err := os.ReadDir("/sys/class/power_supply")
	if err != nil {
		return 0, ""
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "BAT") {
			continue
		}
		base := filepath.Join("/sys/class/power_supply", name)
		capBytes, _ := os.ReadFile(filepath.Join(base, "capacity"))
		statusBytes, _ := os.ReadFile(filepath.Join(base, "status"))
		capacity := strings.TrimSpace(string(capBytes))
		status := strings.TrimSpace(string(statusBytes))
		if capacity != "" {
			percent, _ := strconv.Atoi(capacity)
			return percent, status
		}
	}
	return 0, ""
}

func detectGPU() string {
	entries, err := os.ReadDir("/sys/class/drm")
	if err != nil {
		return ""
	}
	var names []string
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "card") {
			continue
		}
		devicePath := filepath.Join("/sys/class/drm", entry.Name(), "device")
		vendorBytes, _ := os.ReadFile(filepath.Join(devicePath, "vendor_name"))
		if len(vendorBytes) == 0 {
			vendorBytes, _ = os.ReadFile(filepath.Join(devicePath, "vendor"))
		}
		vendor := strings.TrimSpace(string(vendorBytes))
		if vendor == "" {
			continue
		}
		names = append(names, vendor)
	}
	if len(names) == 0 {
		return ""
	}
	seen := make(map[string]bool)
	var unique []string
	for _, name := range names {
		if !seen[name] {
			seen[name] = true
			unique = append(unique, name)
		}
	}
	return strings.Join(unique, ", ")
}

func collectStatus() (statusSnapshot, error) {
	first, err := readCPUTimes()
	if err != nil {
		return statusSnapshot{}, err
	}
	time.Sleep(200 * time.Millisecond)
	second, err := readCPUTimes()
	if err != nil {
		return statusSnapshot{}, err
	}
	var cpuPercent float64
	if delta := second.total - first.total; delta > 0 {
		cpuPercent = 100 * float64(delta-(second.idle-first.idle)) / float64(delta)
	}
	mem := readKeyValues("/proc/meminfo")
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return statusSnapshot{}, err
	}
	hostname, _ := os.Hostname()
	kernelBytes, _ := os.ReadFile("/proc/sys/kernel/osrelease")
	loadBytes, _ := os.ReadFile("/proc/loadavg")
	uptimeBytes, _ := os.ReadFile("/proc/uptime")
	loadFields := strings.Fields(string(loadBytes))
	uptimeFields := strings.Fields(string(uptimeBytes))
	load, uptime := 0.0, 0.0
	if len(loadFields) > 0 {
		load, _ = strconv.ParseFloat(loadFields[0], 64)
	}
	if len(uptimeFields) > 0 {
		uptime, _ = strconv.ParseFloat(uptimeFields[0], 64)
	}
	rx, tx := collectNetwork()
	totalDisk := stat.Blocks * uint64(stat.Bsize)
	freeDisk := stat.Bavail * uint64(stat.Bsize)
	totalMemory := mem["MemTotal"]
	availableMemory := mem["MemAvailable"]
	batteryPercent, batteryStatus := readBattery()
	gpu := detectGPU()
	return statusSnapshot{
		Timestamp: time.Now().Format(time.RFC3339), Hostname: hostname,
		Kernel: strings.TrimSpace(string(kernelBytes)), CPUPercent: cpuPercent, CPUCores: runtime.NumCPU(),
		MemoryUsed: totalMemory - availableMemory, MemoryTotal: totalMemory,
		DiskUsed: totalDisk - freeDisk, DiskTotal: totalDisk, Load1: load, UptimeSeconds: uptime,
		TemperatureC: firstTemperature(), NetworkReceived: rx, NetworkSent: tx,
		BatteryPercent: batteryPercent, BatteryStatus: batteryStatus, GPU: gpu,
	}, nil
}

func (a *app) runStatus(args []string) error {
	jsonOutput := false
	textOutput := false
	if len(args) > 1 {
		return errors.New("usage: raid status [--json|--text]")
	}
	if len(args) == 1 {
		switch args[0] {
		case "--json":
			jsonOutput = true
		case "--text":
			textOutput = true
		default:
			return errors.New("usage: raid status [--json|--text]")
		}
	}
	if !jsonOutput && !textOutput && isInteractiveTerminal() {
		return a.runStatusTUI()
	}
	snapshot, err := collectStatus()
	if err != nil {
		return fmt.Errorf("status requires Linux procfs: %w", err)
	}
	if jsonOutput {
		encoder := json.NewEncoder(a.out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(snapshot)
	}
	fmt.Fprintf(a.out, "Raid status  %s  Linux %s\n", snapshot.Hostname, snapshot.Kernel)
	fmt.Fprintf(a.out, "CPU      %5.1f%%  %d cores  load %.2f\n", snapshot.CPUPercent, snapshot.CPUCores, snapshot.Load1)
	fmt.Fprintf(a.out, "Memory   %s / %s\n", formatBytes(int64(snapshot.MemoryUsed)), formatBytes(int64(snapshot.MemoryTotal)))
	fmt.Fprintf(a.out, "Disk     %s / %s\n", formatBytes(int64(snapshot.DiskUsed)), formatBytes(int64(snapshot.DiskTotal)))
	fmt.Fprintf(a.out, "Network  RX %s  TX %s\n", formatBytes(int64(snapshot.NetworkReceived)), formatBytes(int64(snapshot.NetworkSent)))
	fmt.Fprintf(a.out, "Uptime   %s\n", (time.Duration(snapshot.UptimeSeconds) * time.Second).Round(time.Minute))
	if snapshot.TemperatureC > 0 {
		fmt.Fprintf(a.out, "Thermal  %.1f C\n", snapshot.TemperatureC)
	}
	if snapshot.BatteryPercent > 0 {
		fmt.Fprintf(a.out, "Battery  %d%% %s\n", snapshot.BatteryPercent, snapshot.BatteryStatus)
	}
	if snapshot.GPU != "" {
		fmt.Fprintf(a.out, "GPU      %s\n", snapshot.GPU)
	}
	return nil
}
