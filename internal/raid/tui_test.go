package raid

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func updateMainMenu(t *testing.T, model mainMenuModel, message tea.Msg) mainMenuModel {
	t.Helper()
	updated, _ := model.Update(message)
	result, ok := updated.(mainMenuModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	return result
}

func TestMainMenuPackageInput(t *testing.T) {
	model := mainMenuModel{}
	model = updateMainMenu(t, model, tea.KeyMsg{Type: tea.KeyDown})
	model = updateMainMenu(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if !model.inputMode {
		t.Fatal("expected uninstall package input mode")
	}
	model = updateMainMenu(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("htop")})
	model = updateMainMenu(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if len(model.selected) != 2 || model.selected[0] != "uninstall" || model.selected[1] != "htop" {
		t.Fatalf("unexpected selection: %#v", model.selected)
	}
}

func TestFullScreenCommandsReturnDirectlyToMenu(t *testing.T) {
	for _, command := range []string{"status", "analyze", "analyse"} {
		if !isFullScreenCommand(command) {
			t.Fatalf("expected %s to be full-screen", command)
		}
	}
	for _, command := range []string{"clean", "uninstall", "history"} {
		if isFullScreenCommand(command) {
			t.Fatalf("expected %s to require result pause", command)
		}
	}
}

func TestStatusModelRendersSnapshot(t *testing.T) {
	model := statusModel{width: 100, height: 30, loading: true}
	updated, command := model.Update(statusResultMsg{snapshot: statusSnapshot{
		Timestamp: "2026-01-01T00:00:00Z", Hostname: "ubuntu", Kernel: "6.8",
		CPUPercent: 25, CPUCores: 8, MemoryUsed: 4, MemoryTotal: 16,
		DiskUsed: 50, DiskTotal: 100,
	}})
	result := updated.(statusModel)
	if command == nil {
		t.Fatal("expected next status tick")
	}
	view := result.View()
	if !strings.Contains(view, "LIVE STATUS") || !strings.Contains(view, "ubuntu") {
		t.Fatalf("missing status content: %q", view)
	}
}

func TestAnalyzeModelNavigationAndDeleteConfirmation(t *testing.T) {
	a := testApp(t)
	root := filepath.Join(a.home, "work")
	child := filepath.Join(root, "project")
	model := analyzeModel{app: a, path: root, width: 100, height: 30, loading: true}
	updated, _ := model.Update(analyzeResultMsg{path: root, entries: []pathInfo{{path: child, size: 42, isDir: true}}})
	model = updated.(analyzeModel)
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(analyzeModel)
	if model.path != child || !model.loading || command == nil {
		t.Fatalf("directory navigation failed: path=%q loading=%v", model.path, model.loading)
	}

	model.loading = false
	model.entries = []pathInfo{{path: filepath.Join(child, "build"), size: 42, isDir: true}}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = updated.(analyzeModel)
	if !model.confirming {
		t.Fatal("expected delete confirmation")
	}
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model = updated.(analyzeModel)
	if !model.deleting || command == nil {
		t.Fatal("expected asynchronous Trash command")
	}
}
