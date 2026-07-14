package raid

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type pathInfo struct {
	path    string
	size    int64
	modTime time.Time
	isDir   bool
}

func pathSize(path string) (int64, error) {
	var total int64
	err := filepath.WalkDir(path, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				return fs.SkipDir
			}
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func formatBytes(size int64) string {
	const unit = int64(1024)
	if size < unit {
		return strconv.FormatInt(size, 10) + " B"
	}
	div, exp := unit, 0
	for n := size / unit; n >= unit && exp < 5; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}

func (a *app) runClean(args []string) error {
	flags, rest, err := parseCommonFlags(args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return errors.New("usage: raid clean [--dry-run] [--yes] [--permanent]")
	}
	targetDirs := []string{
		filepath.Join(a.home, ".cache", "thumbnails"),
		filepath.Join(a.home, ".cache", "fontconfig"),
		filepath.Join(a.home, ".cache", "go-build"),
		filepath.Join(a.home, ".cache", "pip", "http-v2"),
		filepath.Join(a.home, ".npm", "_cacache"),
		filepath.Join(a.home, ".cache", "yarn"),
		filepath.Join(a.home, ".cache", "mesa_shader_cache"),
	}
	var targets []string
	for _, dir := range targetDirs {
		if _, err := os.Lstat(dir); err == nil {
			targets = append(targets, dir)
		}
	}
	if len(targets) == 0 {
		fmt.Fprintln(a.out, "No conservative cache targets found.")
		return nil
	}
	fmt.Fprintf(a.out, "Clean plan: %d cache entries\n", len(targets))
	for _, target := range targets {
		fmt.Fprintf(a.out, "PLAN     %s\n", target)
	}
	var failures []error
	for _, target := range targets {
		if err := a.removePath(target, "clean", flags); err != nil {
			fmt.Fprintf(a.errOut, "skip %s: %v\n", target, err)
			failures = append(failures, fmt.Errorf("%s: %w", target, err))
		}
	}
	if !flags.yes {
		fmt.Fprintln(a.out, "Run again with --yes to execute.")
	}
	return errors.Join(failures...)
}

var artifactNames = map[string]bool{
	"node_modules": true, "target": true, "dist": true, "build": true,
	".next": true, ".nuxt": true, ".parcel-cache": true, ".pytest_cache": true,
	"__pycache__": true, ".tox": true, ".gradle": true, "coverage": true,
}

var projectMarkers = []string{
	".git", "package.json", "Cargo.toml", "go.mod", "pyproject.toml",
	"requirements.txt", "pom.xml", "build.gradle", "Package.swift", "CMakeLists.txt", "Makefile",
}

func isProjectRoot(path string) bool {
	for _, marker := range projectMarkers {
		if _, err := os.Lstat(filepath.Join(path, marker)); err == nil {
			return true
		}
	}
	return false
}

func hasMarker(project string, names ...string) bool {
	for _, name := range names {
		info, err := os.Lstat(filepath.Join(project, name))
		if err == nil && info.Mode()&os.ModeSymlink == 0 {
			return true
		}
	}
	return false
}

func artifactBelongsToProject(project, artifact string) bool {
	switch artifact {
	case "node_modules", ".next", ".nuxt", ".parcel-cache", "coverage":
		return hasMarker(project, "package.json")
	case "target":
		return hasMarker(project, "Cargo.toml")
	case ".pytest_cache", "__pycache__", ".tox":
		return hasMarker(project, "pyproject.toml", "requirements.txt")
	case ".gradle":
		return hasMarker(project, "build.gradle")
	case "dist":
		return hasMarker(project, "package.json", "pyproject.toml")
	case "build":
		return hasMarker(project, "pom.xml", "build.gradle", "Package.swift", "CMakeLists.txt")
	default:
		return false
	}
}

func (a *app) runPurge(args []string) error {
	flags, rest, err := parseCommonFlags(args)
	if err != nil {
		return err
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if len(rest) > 1 {
		return errors.New("usage: raid purge [path] [--dry-run] [--yes] [--permanent]")
	}
	if len(rest) == 1 {
		root, err = filepath.Abs(rest[0])
		if err != nil {
			return err
		}
	}
	if _, err := a.validateUserPath(filepath.Join(root, ".raid-probe")); err != nil {
		return fmt.Errorf("unsafe purge root: %w", err)
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		return err
	}
	rootStat, rootStatOK := rootInfo.Sys().(*syscall.Stat_t)

	var targets []string
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fs.SkipDir
		}
		if !entry.IsDir() || path == root {
			return nil
		}
		if rootStatOK {
			info, infoErr := entry.Info()
			if infoErr != nil {
				return fs.SkipDir
			}
			if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Dev != rootStat.Dev {
				return fs.SkipDir
			}
		}
		if entry.Name() == ".git" {
			return fs.SkipDir
		}
		project := nearestProjectRoot(root, filepath.Dir(path))
		if artifactNames[entry.Name()] && project != "" && artifactBelongsToProject(project, entry.Name()) {
			targets = append(targets, path)
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(targets)
	if len(targets) == 0 {
		fmt.Fprintln(a.out, "No rebuildable project artifacts found.")
		return nil
	}
	fmt.Fprintf(a.out, "Purge plan: %d project artifacts under %s\n", len(targets), root)
	for _, target := range targets {
		fmt.Fprintf(a.out, "PLAN     %s\n", target)
	}
	var failures []error
	for _, target := range targets {
		if err := a.removePath(target, "purge", flags); err != nil {
			fmt.Fprintf(a.errOut, "skip %s: %v\n", target, err)
			failures = append(failures, fmt.Errorf("%s: %w", target, err))
		}
	}
	if !flags.yes {
		fmt.Fprintln(a.out, "Run again with --yes to execute.")
	}
	return errors.Join(failures...)
}

func nearestProjectRoot(boundary, start string) string {
	boundary = filepath.Clean(boundary)
	for current := filepath.Clean(start); ; current = filepath.Dir(current) {
		if isProjectRoot(current) {
			return current
		}
		if current == boundary || current == filepath.Dir(current) {
			return ""
		}
		rel, err := filepath.Rel(boundary, current)
		if err != nil || strings.HasPrefix(rel, "..") {
			return ""
		}
	}
}

func (a *app) runInstaller(args []string) error {
	flags, rest, err := parseCommonFlags(args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return errors.New("usage: raid installer [--dry-run] [--yes] [--permanent]")
	}
	roots := []string{filepath.Join(a.home, "Downloads")}
	extensions := map[string]bool{".deb": true, ".appimage": true, ".rpm": true, ".run": true, ".iso": true}
	var installers []pathInfo
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return fs.SkipDir
			}
			if entry.IsDir() {
				rel, _ := filepath.Rel(root, path)
				if rel != "." && strings.Count(filepath.ToSlash(rel), "/") >= 2 {
					return fs.SkipDir
				}
				return nil
			}
			if !extensions[strings.ToLower(filepath.Ext(entry.Name()))] {
				return nil
			}
			info, err := entry.Info()
			if err == nil && info.ModTime().Before(cutoff) {
				installers = append(installers, pathInfo{path: path, size: info.Size(), modTime: info.ModTime()})
			}
			return nil
		})
	}
	sort.Slice(installers, func(i, j int) bool { return installers[i].size > installers[j].size })
	if len(installers) == 0 {
		fmt.Fprintln(a.out, "No installer files found.")
		return nil
	}
	fmt.Fprintln(a.out, "Installer files older than 7 days:")
	for _, item := range installers {
		fmt.Fprintf(a.out, "%9s  %s  %s\n", formatBytes(item.size), item.modTime.Format("2006-01-02"), item.path)
	}
	if !flags.yes {
		fmt.Fprintln(a.out, "Preview only. Use --yes to move every listed installer to Trash.")
		return nil
	}
	var failures []error
	for _, item := range installers {
		if err := a.removePath(item.path, "installer", flags); err != nil {
			fmt.Fprintf(a.errOut, "skip %s: %v\n", item.path, err)
			failures = append(failures, fmt.Errorf("%s: %w", item.path, err))
		}
	}
	return errors.Join(failures...)
}

func (a *app) runHistory(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: raid history")
	}
	lines, err := readLines(filepath.Join(a.stateDir, "operations.tsv"))
	if os.IsNotExist(err) {
		fmt.Fprintln(a.out, "No operation history yet.")
		return nil
	}
	if err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Fprintln(a.out, line)
	}
	return nil
}
