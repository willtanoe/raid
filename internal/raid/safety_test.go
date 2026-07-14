package raid

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testApp(t *testing.T) *app {
	t.Helper()
	t.Setenv("RAID_TEST_MODE", "1")
	home := t.TempDir()
	return &app{home: home, stateDir: filepath.Join(home, "state"), out: os.Stdout, errOut: os.Stderr}
}

func TestValidateUserPathRejectsSymlinkAncestor(t *testing.T) {
	a := testApp(t)
	protected := filepath.Join(a.home, "Documents")
	if err := os.MkdirAll(protected, 0o700); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(a.home, ".cache")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(cache, "thumbnails")
	if err := os.Symlink(protected, link); err != nil {
		t.Fatal(err)
	}
	if _, err := a.validateUserPath(filepath.Join(link, "photo")); err == nil {
		t.Fatal("expected symlink ancestor to be rejected")
	}
}

func TestValidateUserPath(t *testing.T) {
	a := testApp(t)
	allowed := filepath.Join(a.home, ".cache", "thumbnails", "one")
	if _, err := a.validateUserPath(allowed); err != nil {
		t.Fatalf("expected allowed cache path: %v", err)
	}
	for _, path := range []string{"/", a.home, "/tmp/outside", filepath.Join(a.home, ".ssh", "id_ed25519")} {
		if _, err := a.validateUserPath(path); err == nil {
			t.Fatalf("expected protected path: %s", path)
		}
	}
}

func TestRemovePathPreviewsByDefault(t *testing.T) {
	a := testApp(t)
	var output bytes.Buffer
	a.out = &output
	target := filepath.Join(a.home, ".cache", "item")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := a.removePath(target, "test", commonFlags{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("preview removed target: %v", err)
	}
	if !strings.Contains(output.String(), "PREVIEW") {
		t.Fatalf("missing preview output: %q", output.String())
	}
}

func TestMoveToTrashWritesMetadata(t *testing.T) {
	a := testApp(t)
	target := filepath.Join(a.home, ".cache", "item")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.moveToTrash(target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(a.home, ".local", "share", "Trash", "files", "item")); err != nil {
		t.Fatalf("missing trashed file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(a.home, ".local", "share", "Trash", "info", "item.trashinfo")); err != nil {
		t.Fatalf("missing trash metadata: %v", err)
	}
}
