package raid

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNearestProjectRoot(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "app")
	artifactParent := filepath.Join(project, "frontend")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(artifactParent, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := nearestProjectRoot(root, artifactParent); got != project {
		t.Fatalf("got %q, want %q", got, project)
	}
}

func TestNearestProjectRootRejectsUnmarkedTree(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "downloads", "node_modules")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := nearestProjectRoot(root, filepath.Dir(child)); got != "" {
		t.Fatalf("unexpected project root %q", got)
	}
}
