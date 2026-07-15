package raid

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseZshHistory(t *testing.T) {
	input := ": 1234567890:0;ls -la\n: 1234567891:0;cd /tmp\n"
	entries, err := parseZshHistory(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].command != "ls -la" {
		t.Fatalf("got %q", entries[0].command)
	}
	if entries[0].timestamp != 1234567890 {
		t.Fatalf("got %d", entries[0].timestamp)
	}
	if entries[1].command != "cd /tmp" {
		t.Fatalf("got %q", entries[1].command)
	}
}

func TestParseZshHistoryWithSpecialChars(t *testing.T) {
	input := ": 1699000000:0;echo 'hello world'\n: 1699000001:0;git commit -m \"fix: bug\"\n"
	entries, err := parseZshHistory(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].command != "echo 'hello world'" {
		t.Fatalf("got %q", entries[0].command)
	}
	if entries[1].command != `git commit -m "fix: bug"` {
		t.Fatalf("got %q", entries[1].command)
	}
}

func TestParseBashHistory(t *testing.T) {
	input := "ls -la\ncd /tmp\n#1234567890\necho done\n"
	entries, err := parseBashHistory(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].command != "ls -la" {
		t.Fatalf("got %q", entries[0].command)
	}
	if entries[1].command != "cd /tmp" {
		t.Fatalf("got %q", entries[1].command)
	}
	if entries[2].command != "echo done" {
		t.Fatalf("got %q", entries[2].command)
	}
}

func TestParseFishHistory(t *testing.T) {
	input := "- cmd: ls -la\n  when: 1234567890\n- cmd: cd /tmp\n  when: 1234567891\n"
	entries, err := parseFishHistory(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].command != "ls -la" {
		t.Fatalf("got %q", entries[0].command)
	}
	if entries[0].timestamp != 1234567890 {
		t.Fatalf("got %d", entries[0].timestamp)
	}
}

func TestWriteZshHistory(t *testing.T) {
	entries := []historyEntry{
		{command: "ls -la", timestamp: 1234567890},
		{command: "cd /tmp", timestamp: 1234567891},
	}
	var buf bytes.Buffer
	if err := writeZshHistory(entries, &buf); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, ": 1234567890:0;ls -la") {
		t.Fatalf("unexpected zsh output: %q", output)
	}
	if !strings.Contains(output, ": 1234567891:0;cd /tmp") {
		t.Fatalf("unexpected zsh output: %q", output)
	}
}

func TestWriteFishHistory(t *testing.T) {
	entries := []historyEntry{
		{command: "ls -la", timestamp: 1234567890},
	}
	var buf bytes.Buffer
	if err := writeFishHistory(entries, &buf); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "- cmd: ls -la") {
		t.Fatalf("unexpected fish output: %q", output)
	}
	if !strings.Contains(output, "when: 1234567890") {
		t.Fatalf("unexpected fish output: %q", output)
	}
}

func TestWriteBashHistory(t *testing.T) {
	entries := []historyEntry{
		{command: "ls -la", timestamp: 1234567890},
		{command: "cd /tmp", timestamp: 1234567891},
	}
	var buf bytes.Buffer
	if err := writeBashHistory(entries, &buf); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), output)
	}
}

func TestConvertZshToFish(t *testing.T) {
	a := testApp(t)
	var output bytes.Buffer
	a.out = &output

	zshHistoryPath := filepath.Join(a.home, ".zsh_history")
	zshContent := ": 1234567890:0;ls -la\n: 1234567891:0;cd /tmp\n"
	if err := os.WriteFile(zshHistoryPath, []byte(zshContent), 0o600); err != nil {
		t.Fatal(err)
	}

	fishDir := filepath.Join(a.home, ".local", "share", "fish")
	if err := os.MkdirAll(fishDir, 0o700); err != nil {
		t.Fatal(err)
	}
	fishHistoryPath := filepath.Join(fishDir, "fish_history")

	if err := a.runConvert([]string{"zsh-history", "--output", fishHistoryPath, "--yes"}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(fishHistoryPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "- cmd: ls -la") {
		t.Fatalf("expected fish history entry: %q", content)
	}
}

func TestConvertFishToZsh(t *testing.T) {
	a := testApp(t)
	var output bytes.Buffer
	a.out = &output

	fishDir := filepath.Join(a.home, ".local", "share", "fish")
	if err := os.MkdirAll(fishDir, 0o700); err != nil {
		t.Fatal(err)
	}
	fishHistoryPath := filepath.Join(fishDir, "fish_history")
	fishContent := "- cmd: ls -la\n  when: 1234567890\n- cmd: cd /tmp\n  when: 1234567891\n"
	if err := os.WriteFile(fishHistoryPath, []byte(fishContent), 0o600); err != nil {
		t.Fatal(err)
	}

	zshOut := filepath.Join(a.home, "out_zsh_history")
	if err := a.runConvert([]string{"fish-history", "--output", zshOut, "--yes"}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(zshOut)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, ": 1234567890:0;ls -la") {
		t.Fatalf("expected zsh history entry: %q", content)
	}
}

func TestConvertZshToBash(t *testing.T) {
	a := testApp(t)
	var output bytes.Buffer
	a.out = &output

	zshPath := filepath.Join(a.home, ".zsh_history")
	if err := os.WriteFile(zshPath, []byte(": 1699000000:0;ls\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	bashOut := filepath.Join(a.home, "out_bash_history")
	if err := a.runConvert([]string{"zsh-history", "--to", "bash", "--output", bashOut, "--yes"}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(bashOut)
	if !strings.Contains(string(data), "ls") {
		t.Fatalf("expected bash history entry: %q", string(data))
	}
}

func TestConvertPreview(t *testing.T) {
	a := testApp(t)
	var output bytes.Buffer
	a.out = &output

	zshPath := filepath.Join(a.home, ".zsh_history")
	if err := os.WriteFile(zshPath, []byte(": 1234567890:0;echo test\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := a.runConvert([]string{"zsh-history"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Preview only") {
		t.Fatalf("expected preview notice: %s", output.String())
	}
}

func TestConvertJSONPreview(t *testing.T) {
	a := testApp(t)
	var output bytes.Buffer
	a.out = &output

	zshPath := filepath.Join(a.home, ".zsh_history")
	if err := os.WriteFile(zshPath, []byte(": 1234567890:0;ls\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := a.runConvert([]string{"zsh-history", "--json"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"command"`) {
		t.Fatalf("expected JSON output: %s", output.String())
	}
}

func TestConvertInvalidSource(t *testing.T) {
	a := testApp(t)
	var output bytes.Buffer
	a.out = &output

	if err := a.runConvert([]string{"powershell"}); err == nil {
		t.Fatal("expected error for invalid source")
	}
}

func TestConvertInvalidTarget(t *testing.T) {
	a := testApp(t)
	var output bytes.Buffer
	a.out = &output

	zshPath := filepath.Join(a.home, ".zsh_history")
	os.WriteFile(zshPath, []byte(": 1:0;ls\n"), 0o600)

	if err := a.runConvert([]string{"zsh-history", "--to", "powershell"}); err == nil {
		t.Fatal("expected error for invalid target")
	}
}

func TestConvertEmptySource(t *testing.T) {
	a := testApp(t)
	var output bytes.Buffer
	a.out = &output

	if err := a.runConvert(nil); err == nil {
		t.Fatal("expected error for missing source argument")
	}
}

func TestConvertWithCustomInput(t *testing.T) {
	a := testApp(t)
	var output bytes.Buffer
	a.out = &output

	customPath := filepath.Join(a.home, "custom_history")
	if err := os.WriteFile(customPath, []byte(": 1234567890:0;pwd\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	fishOut := filepath.Join(a.home, "fish_out")
	if err := a.runConvert([]string{"zsh-history", "--input", customPath, "--output", fishOut, "--yes"}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(fishOut)
	if !strings.Contains(string(data), "pwd") {
		t.Fatalf("expected pwd in output: %q", string(data))
	}
}

func TestConvertRoundTripZshFishZsh(t *testing.T) {
	a := testApp(t)
	var output bytes.Buffer
	a.out = &output

	original := ": 1699000000:0;ls -la\n: 1699000001:0;echo hello\n"
	zshPath := filepath.Join(a.home, ".zsh_history")
	if err := os.WriteFile(zshPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	fishDir := filepath.Join(a.home, ".local", "share", "fish")
	os.MkdirAll(fishDir, 0o700)
	fishPath := filepath.Join(fishDir, "fish_history")
	if err := a.runConvert([]string{"zsh-history", "--to", "fish", "--output", fishPath, "--yes"}); err != nil {
		t.Fatal(err)
	}

	zshOut := filepath.Join(a.home, "roundtrip_zsh_history")
	if err := a.runConvert([]string{"fish-history", "--to", "zsh", "--input", fishPath, "--output", zshOut, "--yes"}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(zshOut)
	content := string(data)
	if !strings.Contains(content, "ls -la") || !strings.Contains(content, "echo hello") {
		t.Fatalf("roundtrip lost commands: %q", content)
	}
}

func TestMainMenuIncludesConvert(t *testing.T) {
	names := make(map[string]bool)
	for _, item := range mainMenuItems {
		names[item.name] = true
	}
	if !names["Convert"] {
		t.Fatal("expected 'Convert' in main menu")
	}
}
