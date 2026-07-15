package raid

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0, errors.New("empty size")
	}
	multipliers := map[byte]int64{'B': 1, 'K': 1024, 'M': 1024 * 1024, 'G': 1024 * 1024 * 1024, 'T': 1024 * 1024 * 1024 * 1024}
	last := s[len(s)-1]
	if mult, ok := multipliers[last]; ok && len(s) > 1 {
		val, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid size: %s", s)
		}
		return val * mult, nil
	}
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size: %s", s)
	}
	return val, nil
}

func parseAge(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, errors.New("empty age")
	}
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("invalid age: %s", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	if strings.HasSuffix(s, "w") {
		weeks, err := strconv.Atoi(strings.TrimSuffix(s, "w"))
		if err != nil {
			return 0, fmt.Errorf("invalid age: %s", s)
		}
		return time.Duration(weeks) * 7 * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func (a *app) runSearch(args []string) error {
	var (
		minSize   int64
		olderThan time.Duration
		pattern   string
		root      string
		jsonMode  bool
		yes       bool
		permanent bool
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--min-size":
			i++
			if i >= len(args) {
				return errors.New("--min-size requires a value")
			}
			var err error
			minSize, err = parseSize(args[i])
			if err != nil {
				return err
			}
		case "--older-than":
			i++
			if i >= len(args) {
				return errors.New("--older-than requires a value")
			}
			var err error
			olderThan, err = parseAge(args[i])
			if err != nil {
				return err
			}
		case "--pattern":
			i++
			if i >= len(args) {
				return errors.New("--pattern requires a value")
			}
			pattern = args[i]
		case "--path":
			i++
			if i >= len(args) {
				return errors.New("--path requires a value")
			}
			root = args[i]
		case "--json":
			jsonMode = true
		case "--yes", "-y":
			yes = true
		case "--permanent":
			permanent = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("unknown option %q", args[i])
			}
			root = args[i]
		}
	}

	if root == "" {
		root = a.home
	} else {
		resolved, err := filepath.Abs(root)
		if err != nil {
			return err
		}
		root = resolved
	}

	if _, err := a.validateUserPath(filepath.Join(root, ".raid-probe")); err != nil && root != filepath.Clean(a.home) {
		return fmt.Errorf("unsafe search root: %w", err)
	}

	var results []pathInfo
	cutoff := time.Now().Add(-olderThan)

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fs.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil
		}

		if minSize > 0 && info.Size() < minSize {
			return nil
		}
		if olderThan > 0 && info.ModTime().After(cutoff) {
			return nil
		}
		if pattern != "" {
			matched, _ := filepath.Match(pattern, entry.Name())
			if !matched {
				return nil
			}
		}

		results = append(results, pathInfo{path: path, size: info.Size(), modTime: info.ModTime()})
		return nil
	})
	if err != nil {
		return err
	}

	sort.Slice(results, func(i, j int) bool { return results[i].size > results[j].size })

	if len(results) == 0 {
		fmt.Fprintln(a.out, "No matching files found.")
		return nil
	}

	if jsonMode {
		type outputItem struct {
			Path    string `json:"path"`
			Size    int64  `json:"size_bytes"`
			ModTime string `json:"mod_time"`
		}
		output := struct {
			Count   int          `json:"count"`
			Entries []outputItem `json:"entries"`
		}{Count: len(results)}
		for _, item := range results {
			output.Entries = append(output.Entries, outputItem{
				Path: item.path, Size: item.size, ModTime: item.modTime.Format(time.RFC3339),
			})
		}
		encoder := json.NewEncoder(a.out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(output)
	}

	fmt.Fprintf(a.out, "Found %d files:\n", len(results))
	for _, item := range results {
		fmt.Fprintf(a.out, "%10s  %s  %s\n", formatBytes(item.size), item.modTime.Format("2006-01-02"), item.path)
	}

	if !yes {
		fmt.Fprintln(a.out, "Preview only. Use --yes to move all matching files to Trash.")
		return nil
	}

	var failures []error
	for _, item := range results {
		if rmErr := a.removePath(item.path, "search", commonFlags{yes: true, permanent: permanent}); rmErr != nil {
			fmt.Fprintf(a.errOut, "skip %s: %v\n", item.path, rmErr)
			failures = append(failures, fmt.Errorf("%s: %w", item.path, rmErr))
		}
	}
	return errors.Join(failures...)
}
