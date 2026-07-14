package raid

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var protectedHomePaths = []string{
	".ssh",
	".gnupg",
	".pki",
	".password-store",
	".local/share/keyrings",
	".local/share/containers",
	".local/share/docker",
	".local/share/flatpak",
	".config",
	"Documents",
	"Desktop",
	"Pictures",
	"Videos",
	"Music",
}

func (a *app) validateUserPath(path string) (string, error) {
	if path == "" || strings.ContainsRune(path, 0) {
		return "", errors.New("empty or invalid path")
	}
	if !filepath.IsAbs(path) {
		return "", errors.New("path must be absolute")
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return "", errors.New("path traversal is not allowed")
		}
	}
	clean := filepath.Clean(path)
	if clean == "/" || clean == a.home {
		return "", errors.New("filesystem and home roots are protected")
	}
	rel, err := filepath.Rel(a.home, clean)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", errors.New("file operations are restricted to the current home")
	}
	for _, protected := range protectedHomePaths {
		if rel == protected || strings.HasPrefix(rel, protected+string(os.PathSeparator)) {
			return "", fmt.Errorf("protected user data: %s", rel)
		}
	}
	current := a.home
	parentRel, err := filepath.Rel(a.home, filepath.Dir(clean))
	if err != nil {
		return "", err
	}
	if parentRel != "." {
		for _, component := range strings.Split(parentRel, string(os.PathSeparator)) {
			current = filepath.Join(current, component)
			info, statErr := os.Lstat(current)
			if statErr != nil {
				if os.IsNotExist(statErr) {
					break
				}
				return "", statErr
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("symlink ancestor is not allowed: %s", current)
			}
		}
	}
	return clean, nil
}

func (a *app) removePath(path, action string, flags commonFlags) error {
	if os.Geteuid() == 0 && os.Getenv("RAID_TEST_MODE") != "1" {
		return errors.New("refusing user-file removal as root; run Raid as your normal user")
	}
	clean, err := a.validateUserPath(path)
	if err != nil {
		return err
	}
	info, err := os.Lstat(clean)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		parentInfo, parentErr := os.Stat(filepath.Dir(clean))
		if parentErr != nil {
			return parentErr
		}
		targetStat, targetOK := info.Sys().(*syscall.Stat_t)
		parentStat, parentOK := parentInfo.Sys().(*syscall.Stat_t)
		if targetOK && parentOK && targetStat.Dev != parentStat.Dev {
			return fmt.Errorf("refusing mountpoint target: %s", clean)
		}
	}
	// Revalidate immediately before mutation to narrow path-swap races.
	if _, err := a.validateUserPath(clean); err != nil {
		return err
	}
	mode := "trash"
	if flags.permanent {
		mode = "permanent"
	}
	if !flags.yes {
		fmt.Fprintf(a.out, "PREVIEW  %-9s %s\n", mode, clean)
		return nil
	}
	if flags.permanent {
		err = os.RemoveAll(clean)
	} else {
		err = a.moveToTrash(clean)
	}
	if err != nil {
		a.logOperation("FAILED", action, clean, err.Error())
		return err
	}
	fmt.Fprintf(a.out, "REMOVED  %s\n", clean)
	a.logOperation(strings.ToUpper(mode), action, clean, "")
	return nil
}

func (a *app) moveToTrash(path string) error {
	trashRoot := filepath.Join(a.home, ".local", "share", "Trash")
	filesDir := filepath.Join(trashRoot, "files")
	infoDir := filepath.Join(trashRoot, "info")
	for _, dir := range []string{trashRoot, filesDir, infoDir} {
		info, err := os.Lstat(dir)
		if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
			return fmt.Errorf("unsafe Trash directory: %s", dir)
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.MkdirAll(filesDir, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(infoDir, 0o700); err != nil {
		return err
	}
	for _, dir := range []string{trashRoot, filesDir, infoDir} {
		info, err := os.Lstat(dir)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("unsafe Trash directory: %s", dir)
		}
	}

	base := filepath.Base(path)
	name := base
	for i := 1; ; i++ {
		_, fileErr := os.Lstat(filepath.Join(filesDir, name))
		_, infoErr := os.Lstat(filepath.Join(infoDir, name+".trashinfo"))
		if os.IsNotExist(fileErr) && os.IsNotExist(infoErr) {
			break
		}
		name = fmt.Sprintf("%s.%d", base, i)
		if i >= 1000 {
			return errors.New("could not allocate a unique Trash name")
		}
	}
	destination := filepath.Join(filesDir, name)
	infoPath := filepath.Join(infoDir, name+".trashinfo")
	infoFile, err := os.OpenFile(infoPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("reserve Trash metadata: %w", err)
	}
	if err := os.Rename(path, destination); err != nil {
		_ = infoFile.Close()
		_ = os.Remove(infoPath)
		if errors.Is(err, syscall.EXDEV) {
			return errors.New("Trash is on another filesystem; refusing permanent fallback")
		}
		return err
	}

	escaped := (&url.URL{Path: path}).EscapedPath()
	content := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n", escaped, time.Now().Format("2006-01-02T15:04:05"))
	if _, err := infoFile.WriteString(content); err != nil {
		_ = infoFile.Close()
		_ = os.Rename(destination, path)
		_ = os.Remove(infoPath)
		return fmt.Errorf("write Trash metadata: %w", err)
	}
	if err := infoFile.Close(); err != nil {
		return fmt.Errorf("close Trash metadata: %w", err)
	}
	return nil
}

func (a *app) logOperation(status, action, path, detail string) {
	if err := os.MkdirAll(a.stateDir, 0o700); err != nil {
		return
	}
	file, err := os.OpenFile(filepath.Join(a.stateDir, "operations.tsv"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	clean := func(value string) string {
		return strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace(value)
	}
	_, _ = fmt.Fprintf(file, "%s\t%s\t%s\t%s\t%s\n", time.Now().Format(time.RFC3339), clean(status), clean(action), clean(path), clean(detail))
}

func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}
