package raid

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestLoadConfigEmpty(t *testing.T) {
	cfg := loadConfig(t.TempDir())
	if len(cfg.AdditionalCacheDirs) != 0 {
		t.Fatalf("expected zero additional cache dirs, got %d", len(cfg.AdditionalCacheDirs))
	}
}

func TestLoadConfigParses(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "raid")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	content := "# comment\nadditional-cache-dir=/tmp/cache1\nadditional-cache-dir=/tmp/cache2\n\n"
	if err := os.WriteFile(filepath.Join(configDir, "config"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := loadConfig(home)
	if len(cfg.AdditionalCacheDirs) != 2 {
		t.Fatalf("expected 2 additional cache dirs, got %d", len(cfg.AdditionalCacheDirs))
	}
	if cfg.AdditionalCacheDirs[0] != "/tmp/cache1" {
		t.Fatalf("got %q, want /tmp/cache1", cfg.AdditionalCacheDirs[0])
	}
	if cfg.AdditionalCacheDirs[1] != "/tmp/cache2" {
		t.Fatalf("got %q, want /tmp/cache2", cfg.AdditionalCacheDirs[1])
	}
}

func TestConfigIntegratedInClean(t *testing.T) {
	a := testApp(t)
	configDir := filepath.Join(a.home, ".config", "raid")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	extraCache := filepath.Join(a.home, "extra-cache")
	if err := os.MkdirAll(extraCache, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config"), []byte("additional-cache-dir="+extraCache+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.config = loadConfig(a.home)
	var output bytes.Buffer
	a.out = &output
	if err := a.runClean(nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), extraCache) {
		t.Fatalf("expected extra cache dir in output, got: %s", output.String())
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"1000", 1000},
		{"10K", 10 * 1024},
		{"1M", 1024 * 1024},
		{"1G", 1024 * 1024 * 1024},
	}
	for _, tc := range tests {
		got, err := parseSize(tc.input)
		if err != nil {
			t.Fatalf("parseSize(%q) error: %v", tc.input, err)
		}
		if got != tc.expected {
			t.Fatalf("parseSize(%q) = %d, want %d", tc.input, got, tc.expected)
		}
	}
	_, err := parseSize("")
	if err == nil {
		t.Fatal("expected error for empty size")
	}
}

func TestParseAge(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"7d", 7 * 24 * time.Hour},
		{"2w", 14 * 24 * time.Hour},
		{"24h", 24 * time.Hour},
	}
	for _, tc := range tests {
		got, err := parseAge(tc.input)
		if err != nil {
			t.Fatalf("parseAge(%q) error: %v", tc.input, err)
		}
		if got != tc.expected {
			t.Fatalf("parseAge(%q) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

func TestSearchFindsFiles(t *testing.T) {
	a := testApp(t)
	var output bytes.Buffer
	a.out = &output

	workDir := filepath.Join(a.home, "work")
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		t.Fatal(err)
	}
	smallFile := filepath.Join(workDir, "small.txt")
	if err := os.WriteFile(smallFile, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	largeFile := filepath.Join(workDir, "large.bin")
	buf := make([]byte, 1024*1024)
	if err := os.WriteFile(largeFile, buf, 0o600); err != nil {
		t.Fatal(err)
	}

	// Search by min size
	output.Reset()
	if err := a.runSearch([]string{"--path", workDir, "--min-size", "500K"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "large.bin") {
		t.Fatalf("expected large.bin in output: %s", output.String())
	}
	if strings.Contains(output.String(), "small.txt") {
		t.Fatal("small.txt should not match min-size filter")
	}

	// Search by pattern
	output.Reset()
	if err := a.runSearch([]string{"--path", workDir, "--pattern", "*.txt"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "small.txt") {
		t.Fatalf("expected small.txt in pattern output: %s", output.String())
	}

	// JSON output
	output.Reset()
	if err := a.runSearch([]string{"--path", workDir, "--json"}); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Count   int `json:"count"`
		Entries []struct {
			Path string `json:"path"`
			Size int64  `json:"size_bytes"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, output.String())
	}
	if result.Count < 2 {
		t.Fatalf("expected at least 2 entries, got %d", result.Count)
	}
}

func TestSearchRejectsUnsafePath(t *testing.T) {
	a := testApp(t)
	var output bytes.Buffer
	a.out = &output

	if err := a.runSearch([]string{"--path", "/"}); err == nil {
		t.Fatal("expected error for filesystem root")
	}
}

func TestMatchInstallerExt(t *testing.T) {
	simple := map[string]bool{".deb": true, ".gz": true, ".xz": true, ".zip": true}
	compound := []string{".tar.gz", ".tar.xz", ".tar.bz2"}

	if !matchInstallerExt("package.deb", simple, compound) {
		t.Fatal("expected .deb to match")
	}
	if !matchInstallerExt("archive.tar.gz", simple, compound) {
		t.Fatal("expected .tar.gz to match")
	}
	if !matchInstallerExt("archive.tar.xz", simple, compound) {
		t.Fatal("expected .tar.xz to match")
	}
	if matchInstallerExt("readme.txt", simple, compound) {
		t.Fatal("expected .txt not to match")
	}
}

func TestRunHistoryFiltering(t *testing.T) {
	a := testApp(t)

	// Write some test operations
	if err := os.MkdirAll(a.stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		"2025-07-01T10:00:00Z\tTRASH\tclean\t/home/user/cache\t",
		"2025-07-02T10:00:00Z\tEXECUTED\tuninstall\tfirefox\t",
		"2025-07-10T10:00:00Z\tTRASH\tclean\t/home/user/other\t",
	}
	if err := os.WriteFile(filepath.Join(a.stateDir, "operations.tsv"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	a.out = &output

	// Filter by command
	output.Reset()
	if err := a.runHistory([]string{"--command", "uninstall"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "firefox") {
		t.Fatalf("expected firefox in output: %s", output.String())
	}
	if strings.Count(output.String(), "\n") > 2 {
		t.Fatal("expected only one filtered entry")
	}

	// Filter by since
	output.Reset()
	if err := a.runHistory([]string{"--since", "2025-07-05"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "2025-07-01") || strings.Contains(output.String(), "2025-07-02") {
		t.Fatal("expected older entries to be filtered out")
	}

	// JSON output
	output.Reset()
	if err := a.runHistory([]string{"--json"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.TrimSpace(output.String()), "[") {
		t.Fatalf("expected JSON array output: %s", output.String())
	}
}

func TestRunDockerNotInstalled(t *testing.T) {
	a := testApp(t)
	var output bytes.Buffer
	a.out = &output

	if commandExists("docker") {
		t.Skip("docker is installed; skipping docker-absence test")
	}

	if err := a.runDocker(nil); err == nil {
		t.Fatal("expected error when docker is not installed")
	}
}

func TestRunUpdatePreview(t *testing.T) {
	a := testApp(t)
	var output bytes.Buffer
	a.out = &output

	if err := a.runUpdate(nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Preview only") {
		t.Fatalf("expected preview notice: %s", output.String())
	}
}

func TestRunCleanIncludesCustomCacheDirs(t *testing.T) {
	a := testApp(t)
	extra := filepath.Join(a.home, "custom-cache")
	if err := os.MkdirAll(extra, 0o700); err != nil {
		t.Fatal(err)
	}
	a.config = config{AdditionalCacheDirs: []string{extra}}

	var output bytes.Buffer
	a.out = &output
	if err := a.runClean([]string{"--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), extra) {
		t.Fatalf("expected custom cache dir in plan: %s", output.String())
	}
}

func TestVersion(t *testing.T) {
	if Version == "" {
		t.Fatal("version should not be empty")
	}
}

func TestMainMenuIncludesNewItems(t *testing.T) {
	names := make(map[string]bool)
	for _, item := range mainMenuItems {
		names[item.name] = true
	}
	for _, want := range []string{"Update", "Docker", "Search"} {
		if !names[want] {
			t.Fatalf("expected menu item %q", want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		size     int64
		expected string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
	}
	for _, tc := range tests {
		got := formatBytes(tc.size)
		if got != tc.expected {
			t.Fatalf("formatBytes(%d) = %q, want %q", tc.size, got, tc.expected)
		}
	}
}

func TestDetectGPUReturnsString(t *testing.T) {
	gpu := detectGPU()
	_ = gpu // may be empty if no GPU available
}

func TestReadBatteryReturnsValues(t *testing.T) {
	percent, status := readBattery()
	_ = percent
	_ = status
}

func TestIsFullScreenCommandWithNewCommands(t *testing.T) {
	for _, command := range []string{"update", "docker", "search"} {
		if isFullScreenCommand(command) {
			t.Fatalf("%s should not be full-screen", command)
		}
	}
}

func TestParseCommonFlagsRejectsPermanentForDocker(t *testing.T) {
	a := testApp(t)
	var output bytes.Buffer
	a.out = &output

	if err := a.runDocker([]string{"--permanent"}); err == nil {
		t.Fatal("expected --permanent to be rejected for docker")
	}
}

func TestParseCommonFlagsRejectsPermanentForUpdate(t *testing.T) {
	a := testApp(t)
	var output bytes.Buffer
	a.out = &output

	if err := a.runUpdate([]string{"--permanent"}); err == nil {
		t.Fatal("expected --permanent to be rejected for update")
	}
}

func TestSearchJSONOutput(t *testing.T) {
	a := testApp(t)
	var output bytes.Buffer
	a.out = &output

	workDir := filepath.Join(a.home, "data")
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "one.txt"), make([]byte, 512), 0o600); err != nil {
		t.Fatal(err)
	}

	output.Reset()
	if err := a.runSearch([]string{"--path", workDir, "--json"}); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Count   int `json:"count"`
		Entries []struct {
			Path    string `json:"path"`
			Size    int64  `json:"size_bytes"`
			ModTime string `json:"mod_time"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result.Count != 1 {
		t.Fatalf("expected 1 entry, got %d", result.Count)
	}
}

func TestNearestProjectRootWithMarkers(t *testing.T) {
	// Test go.mod marker
	root := t.TempDir()
	goProject := filepath.Join(root, "goapp")
	if err := os.MkdirAll(goProject, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goProject, "go.mod"), []byte("module test"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(goProject, "vendor")
	if err := os.MkdirAll(artifact, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := nearestProjectRoot(root, artifact); got != goProject {
		t.Fatalf("got %q, want %q", got, goProject)
	}
}

func TestArtifactBelongsToProject(t *testing.T) {
	tests := []struct {
		artifact string
		markers  []string
		expected bool
	}{
		{"node_modules", []string{"package.json"}, true},
		{"target", []string{"Cargo.toml"}, true},
		{"node_modules", []string{"Cargo.toml"}, false},
	}
	for _, tc := range tests {
		root := t.TempDir()
		for _, marker := range tc.markers {
			if err := os.WriteFile(filepath.Join(root, marker), []byte(""), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if got := artifactBelongsToProject(root, tc.artifact); got != tc.expected {
			t.Fatalf("artifactBelongsToProject(%s) = %v, want %v", tc.artifact, got, tc.expected)
		}
	}
}

func TestNewAppLoadsConfig(t *testing.T) {
	t.Setenv("RAID_TEST_MODE", "1")
	home := t.TempDir()
	t.Setenv("RAID_HOME", home)
	stateDir := filepath.Join(home, "state")
	t.Setenv("RAID_STATE_DIR", stateDir)

	configDir := filepath.Join(home, ".config", "raid")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config"), []byte("additional-cache-dir=/tmp/test-cache\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	app, err := newApp()
	if err != nil {
		t.Fatal(err)
	}
	if len(app.config.AdditionalCacheDirs) != 1 {
		t.Fatalf("expected 1 additional cache dir, got %d", len(app.config.AdditionalCacheDirs))
	}
	if app.config.AdditionalCacheDirs[0] != "/tmp/test-cache" {
		t.Fatalf("got %q", app.config.AdditionalCacheDirs[0])
	}
}

func TestStatusSnapshotIncludesOptionalFields(t *testing.T) {
	snap := statusSnapshot{
		BatteryPercent: 85,
		BatteryStatus:  "Discharging",
		GPU:            "Intel UHD Graphics",
	}
	data, _ := json.Marshal(snap)
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	if result["battery_percent"] != float64(85) {
		t.Fatal("missing battery_percent in JSON")
	}
	if result["battery_status"] != "Discharging" {
		t.Fatal("missing battery_status in JSON")
	}
	if result["gpu"] != "Intel UHD Graphics" {
		t.Fatal("missing gpu in JSON")
	}
}

func TestParseCommonFlagsPermutation(t *testing.T) {
	flags, rest, err := parseCommonFlags([]string{"--dry-run", "arg1"})
	if err != nil {
		t.Fatal(err)
	}
	if !flags.dryRun {
		t.Fatal("expected --dry-run")
	}
	if flags.yes {
		t.Fatal("--yes should not be set with --dry-run")
	}
	if len(rest) != 1 || rest[0] != "arg1" {
		t.Fatalf("unexpected rest: %v", rest)
	}
}

func TestSortBySize(t *testing.T) {
	items := []pathInfo{
		{path: "/small", size: 100, isDir: false},
		{path: "/large", size: 1000, isDir: false},
		{path: "/medium", size: 500, isDir: false},
	}
	sort.Slice(items, func(i, j int) bool { return items[i].size > items[j].size })
	if items[0].path != "/large" || items[1].path != "/medium" || items[2].path != "/small" {
		t.Fatalf("unexpected sort order: %v", items)
	}
}

func TestRequireArgs(t *testing.T) {
	if err := requireArgs([]string{}, "usage: test"); err == nil {
		t.Fatal("expected error for empty args")
	}
	if err := requireArgs([]string{"ok"}, "usage: test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
