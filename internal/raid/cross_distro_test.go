package raid

import (
	"bytes"
	"strings"
	"testing"
)

func TestDetectPackageHasDnf(t *testing.T) {
	if !commandExists("rpm") {
		t.Skip("rpm/dnf not available")
	}
	matches := detectPackage("nosuchpackage_xyz_123")
	if len(matches) != 0 {
		t.Fatalf("expected no matches for nonexistent package, got %v", matches)
	}
}

func TestDetectPackageHasPacman(t *testing.T) {
	if !commandExists("pacman") {
		t.Skip("pacman not available")
	}
	matches := detectPackage("nosuchpackage_xyz_123")
	if len(matches) != 0 {
		t.Fatalf("expected no matches for nonexistent package, got %v", matches)
	}
}

func TestDetectPackageNoManagers(t *testing.T) {
	matches := detectPackage("nosuchpackage_xyz_123")
	if len(matches) != 0 {
		t.Fatalf("expected no matches for nonexistent package, got %v", matches)
	}
}

func TestRunUpdateListsDnfWhenAvailable(t *testing.T) {
	if !commandExists("dnf") {
		t.Skip("dnf not available")
	}
	a := testApp(t)
	var output bytes.Buffer
	a.out = &output
	if err := a.runUpdate([]string{"--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "DNF") {
		t.Fatal("expected DNF in update plan")
	}
}

func TestRunUpdateListsPacmanWhenAvailable(t *testing.T) {
	if !commandExists("pacman") {
		t.Skip("pacman not available")
	}
	a := testApp(t)
	var output bytes.Buffer
	a.out = &output
	if err := a.runUpdate([]string{"--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Pacman") {
		t.Fatal("expected Pacman in update plan")
	}
}
