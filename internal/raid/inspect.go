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

type ProcessInfo struct {
	PID         int     `json:"pid"`
	Name        string  `json:"name"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemoryBytes uint64  `json:"memory_bytes"`
}

type statusSnapshot struct {
	Timestamp       string        `json:"timestamp"`
	Hostname        string        `json:"hostname"`
	Kernel          string        `json:"kernel"`
	CPUPercent      float64       `json:"cpu_percent"`
	CPUCores        int           `json:"cpu_cores"`
	PerCoreCPU      []float64     `json:"per_core_cpu,omitempty"`
	MemoryUsed      uint64        `json:"memory_used_bytes"`
	MemoryTotal     uint64        `json:"memory_total_bytes"`
	SwapUsed        uint64        `json:"swap_used_bytes"`
	SwapTotal       uint64        `json:"swap_total_bytes"`
	DiskUsed        uint64        `json:"disk_used_bytes"`
	DiskTotal       uint64        `json:"disk_total_bytes"`
	DiskReadRate    float64       `json:"disk_read_mbps,omitempty"`
	DiskWriteRate   float64       `json:"disk_write_mbps,omitempty"`
	Load1           float64       `json:"load_1"`
	UptimeSeconds   float64       `json:"uptime_seconds"`
	TemperatureC    float64       `json:"temperature_c,omitempty"`
	NetworkReceived uint64        `json:"network_received_bytes"`
	NetworkSent     uint64        `json:"network_sent_bytes"`
	BatteryPercent  int           `json:"battery_percent,omitempty"`
	BatteryStatus   string        `json:"battery_status,omitempty"`
	GPU             string        `json:"gpu,omitempty"`
	TopProcesses    []ProcessInfo `json:"top_processes,omitempty"`
	HealthScore     int           `json:"health_score"`
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

func collectPerCoreCPU() []float64 {
	type coreTime struct {
		idle  uint64
		total uint64
	}
	readCores := func() map[string]coreTime {
		result := make(map[string]coreTime)
		file, err := os.Open("/proc/stat")
		if err != nil {
			return result
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "cpu") || len(line) < 5 {
				continue
			}
			if strings.HasPrefix(line, "cpu ") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 5 {
				continue
			}
			var values []uint64
			for _, f := range fields[1:] {
				v, _ := strconv.ParseUint(f, 10, 64)
				values = append(values, v)
			}
			if len(values) < 4 {
				continue
			}
			var total uint64
			for _, v := range values {
				total += v
			}
			idle := values[3]
			if len(values) > 4 {
				idle += values[4]
			}
			result[fields[0]] = coreTime{idle: idle, total: total}
		}
		return result
	}

	first := readCores()
	time.Sleep(200 * time.Millisecond)
	second := readCores()

	var perCore []float64
	for core := 0; core < runtime.NumCPU(); core++ {
		name := fmt.Sprintf("cpu%d", core)
		f, ok1 := first[name]
		s, ok2 := second[name]
		if !ok1 || !ok2 {
			perCore = append(perCore, -1)
			continue
		}
		delta := s.total - f.total
		if delta > 0 {
			pct := 100 * float64(delta-(s.idle-f.idle)) / float64(delta)
			perCore = append(perCore, pct)
		} else {
			perCore = append(perCore, 0)
		}
	}
	return perCore
}

var prevDiskRead, prevDiskWrite uint64
var prevDiskTime time.Time

func readDiskIO() (float64, float64) {
	file, err := os.Open("/proc/diskstats")
	if err != nil {
		return 0, 0
	}
	defer file.Close()
	var reads, writes uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 14 {
			continue
		}
		r, _ := strconv.ParseUint(fields[5], 10, 64)
		w, _ := strconv.ParseUint(fields[9], 10, 64)
		reads += r
		writes += w
	}
	now := time.Now()
	var readRate, writeRate float64
	if !prevDiskTime.IsZero() {
		elapsed := now.Sub(prevDiskTime).Seconds()
		if elapsed > 0 {
			if reads >= prevDiskRead {
				readRate = float64(reads-prevDiskRead) * 512 / elapsed / 1024 / 1024
			}
			if writes >= prevDiskWrite {
				writeRate = float64(writes-prevDiskWrite) * 512 / elapsed / 1024 / 1024
			}
		}
	}
	prevDiskRead, prevDiskWrite = reads, writes
	prevDiskTime = now
	return readRate, writeRate
}

func readSwap() (uint64, uint64) {
	mem := readKeyValues("/proc/meminfo")
	return mem["MemTotal"] - mem["MemAvailable"], mem["SwapTotal"] - mem["SwapFree"]
}

func readTopProcesses() []ProcessInfo {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	type procSample struct {
		pid   int
		name  string
		utime uint64
		stime uint64
		rss   uint64
	}
	var procs []procSample
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == 0 {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if err != nil {
			continue
		}
		content := string(data)
		closeParen := strings.LastIndex(content, ")")
		if closeParen < 0 {
			continue
		}
		fields := strings.Fields(content[closeParen+2:])
		if len(fields) < 20 {
			continue
		}
		utime, _ := strconv.ParseUint(fields[11], 10, 64)
		stime, _ := strconv.ParseUint(fields[12], 10, 64)
		rss, _ := strconv.ParseUint(fields[21], 10, 64)
		name := strings.Trim(content[strings.Index(content, "(")+1:closeParen], " ")
		procs = append(procs, procSample{pid: pid, name: name, utime: utime, stime: stime, rss: rss * uint64(os.Getpagesize())})
	}
	time.Sleep(100 * time.Millisecond)

	var results []ProcessInfo
	for _, p := range procs {
		data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(p.pid), "stat"))
		if err != nil {
			continue
		}
		content := string(data)
		closeParen := strings.LastIndex(content, ")")
		if closeParen < 0 {
			continue
		}
		fields := strings.Fields(content[closeParen+2:])
		if len(fields) < 20 {
			continue
		}
		utime2, _ := strconv.ParseUint(fields[11], 10, 64)
		stime2, _ := strconv.ParseUint(fields[12], 10, 64)
		cpuDelta := (utime2 - p.utime) + (stime2 - p.stime)
		if cpuDelta < 2 {
			continue
		}
		cpuPct := float64(cpuDelta) / 10.0
		results = append(results, ProcessInfo{
			PID: p.pid, Name: p.name, CPUPercent: cpuPct, MemoryBytes: p.rss,
		})
	}

	sort.Slice(results, func(i, j int) bool { return results[i].CPUPercent > results[j].CPUPercent })
	if len(results) > 5 {
		results = results[:5]
	}
	return results
}

func computeHealthScore(s statusSnapshot) int {
	score := 100
	if s.CPUPercent > 90 {
		score -= 25
	} else if s.CPUPercent > 70 {
		score -= 10
	}
	if s.MemoryTotal > 0 {
		memPct := float64(s.MemoryUsed) / float64(s.MemoryTotal) * 100
		if memPct > 95 {
			score -= 25
		} else if memPct > 80 {
			score -= 10
		}
	}
	if s.DiskTotal > 0 {
		diskPct := float64(s.DiskUsed) / float64(s.DiskTotal) * 100
		if diskPct > 95 {
			score -= 25
		} else if diskPct > 80 {
			score -= 10
		}
	}
	if s.TemperatureC >= 90 {
		score -= 20
	} else if s.TemperatureC >= 70 {
		score -= 5
	}
	if s.BatteryPercent > 0 && s.BatteryPercent < 10 && s.BatteryStatus != "Charging" {
		score -= 15
	}
	if score < 0 {
		score = 0
	}
	return score
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
	swapTotal := mem["SwapTotal"]
	swapFree := mem["SwapFree"]
	batteryPercent, batteryStatus := readBattery()
	gpu := detectGPU()
	diskRead, diskWrite := readDiskIO()
	perCore := collectPerCoreCPU()
	snap := statusSnapshot{
		Timestamp: time.Now().Format(time.RFC3339), Hostname: hostname,
		Kernel: strings.TrimSpace(string(kernelBytes)), CPUPercent: cpuPercent, CPUCores: runtime.NumCPU(),
		PerCoreCPU: perCore,
		MemoryUsed: totalMemory - availableMemory, MemoryTotal: totalMemory,
		SwapUsed: swapTotal - swapFree, SwapTotal: swapTotal,
		DiskUsed: totalDisk - freeDisk, DiskTotal: totalDisk,
		DiskReadRate: diskRead, DiskWriteRate: diskWrite,
		Load1: load, UptimeSeconds: uptime,
		TemperatureC: firstTemperature(), NetworkReceived: rx, NetworkSent: tx,
		BatteryPercent: batteryPercent, BatteryStatus: batteryStatus, GPU: gpu,
	}
	snap.HealthScore = computeHealthScore(snap)
	return snap, nil
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
	fmt.Fprintf(a.out, "Health   %d/100\n", snapshot.HealthScore)
	fmt.Fprintf(a.out, "CPU      %5.1f%%  %d cores  load %.2f\n", snapshot.CPUPercent, snapshot.CPUCores, snapshot.Load1)
	fmt.Fprintf(a.out, "Memory   %s / %s", formatBytes(int64(snapshot.MemoryUsed)), formatBytes(int64(snapshot.MemoryTotal)))
	if snapshot.SwapTotal > 0 {
		fmt.Fprintf(a.out, "  (swap %s / %s)", formatBytes(int64(snapshot.SwapUsed)), formatBytes(int64(snapshot.SwapTotal)))
	}
	fmt.Fprintln(a.out)
	fmt.Fprintf(a.out, "Disk     %s / %s", formatBytes(int64(snapshot.DiskUsed)), formatBytes(int64(snapshot.DiskTotal)))
	if snapshot.DiskReadRate > 0 || snapshot.DiskWriteRate > 0 {
		fmt.Fprintf(a.out, "  (R %.1f MB/s  W %.1f MB/s)", snapshot.DiskReadRate, snapshot.DiskWriteRate)
	}
	fmt.Fprintln(a.out)
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
	if len(snapshot.TopProcesses) > 0 {
		fmt.Fprintln(a.out, "Processes")
		for _, p := range snapshot.TopProcesses[:minInt(3, len(snapshot.TopProcesses))] {
			fmt.Fprintf(a.out, "  %5.1f%%  %s (%d)\n", p.CPUPercent, p.Name, p.PID)
		}
	}
	return nil
}
