package raid

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type historyEntry struct {
	command   string
	timestamp int64
}

var zshMetaPattern = regexp.MustCompile(`^:\s*(\d+):\d+;(.*)`)

func parseZshHistory(reader io.Reader) ([]historyEntry, error) {
	var entries []historyEntry
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		matches := zshMetaPattern.FindStringSubmatch(line)
		if len(matches) == 3 {
			ts, err := strconv.ParseInt(matches[1], 10, 64)
			if err != nil {
				ts = time.Now().Unix()
			}
			entries = append(entries, historyEntry{
				timestamp: ts,
				command:   matches[2],
			})
		}
	}
	return entries, scanner.Err()
}

func parseBashHistory(reader io.Reader) ([]historyEntry, error) {
	var entries []historyEntry
	scanner := bufio.NewScanner(reader)
	ts := time.Now().Unix() - 600
	for i := 0; scanner.Scan(); i++ {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			if t, err := strconv.ParseInt(strings.TrimPrefix(line, "#"), 10, 64); err == nil {
				ts = t
			}
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		entries = append(entries, historyEntry{
			timestamp: ts + int64(i),
			command:   line,
		})
	}
	return entries, scanner.Err()
}

func parseFishHistory(reader io.Reader) ([]historyEntry, error) {
	var entries []historyEntry
	scanner := bufio.NewScanner(reader)
	var current historyEntry
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "- cmd:") {
			if current.command != "" {
				entries = append(entries, current)
			}
			current = historyEntry{command: strings.TrimPrefix(line, "- cmd: ")}
		} else if strings.HasPrefix(line, "  when:") {
			current.timestamp, _ = strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "  when:")), 10, 64)
		}
	}
	if current.command != "" {
		entries = append(entries, current)
	}
	return entries, scanner.Err()
}

func writeZshHistory(entries []historyEntry, writer io.Writer) error {
	for _, e := range entries {
		ts := e.timestamp
		if ts == 0 {
			ts = time.Now().Unix()
		}
		fmt.Fprintf(writer, ": %d:0;%s\n", ts, e.command)
	}
	return nil
}

func writeFishHistory(entries []historyEntry, writer io.Writer) error {
	for _, e := range entries {
		fmt.Fprintf(writer, "- cmd: %s\n  when: %d\n", e.command, e.timestamp)
	}
	return nil
}

func writeBashHistory(entries []historyEntry, writer io.Writer) error {
	for _, e := range entries {
		fmt.Fprintln(writer, e.command)
	}
	return nil
}

func (a *app) runConvert(args []string) error {
	var (
		source      string
		target      string
		inputFile   string
		outputFile  string
		yes         bool
		jsonPreview bool
	)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--to":
			i++
			if i >= len(args) {
				return errors.New("--to requires a value (zsh|fish|bash)")
			}
			target = args[i]
		case "--input", "-i":
			i++
			if i >= len(args) {
				return errors.New("--input requires a file path")
			}
			inputFile = args[i]
		case "--output", "-o":
			i++
			if i >= len(args) {
				return errors.New("--output requires a file path")
			}
			outputFile = args[i]
		case "--yes", "-y":
			yes = true
		case "--json":
			jsonPreview = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("unknown option %q", args[i])
			}
			if source != "" {
				return errors.New("usage: raid convert <source> [--to <format>] [--input <file>] [--output <file>] [--yes]")
			}
			source = args[i]
		}
	}

	if source == "" {
		return errors.New("usage: raid convert <zsh-history|fish-history|bash-history> [--to zsh|fish|bash] [--input <file>] [--output <file>] [--yes]")
	}

	switch source {
	case "zsh-history", "zsh":
		if inputFile == "" {
			inputFile = filepath.Join(a.home, ".zsh_history")
		}
		if target == "" {
			target = "fish"
		}
	case "fish-history", "fish":
		if inputFile == "" {
			inputFile = filepath.Join(a.home, ".local", "share", "fish", "fish_history")
		}
		if target == "" {
			target = "zsh"
		}
	case "bash-history", "bash":
		if inputFile == "" {
			inputFile = filepath.Join(a.home, ".bash_history")
		}
		if target == "" {
			target = "zsh"
		}
	default:
		return fmt.Errorf("unknown source %q; expected zsh-history, fish-history, or bash-history", source)
	}

	switch target {
	case "zsh", "fish", "bash":
	default:
		return fmt.Errorf("unknown target format %q; expected zsh, fish, or bash", target)
	}

	sourceName := strings.TrimSuffix(source, "-history")
	targetName := target

	entries, err := loadHistoryEntries(inputFile, sourceName)
	if err != nil {
		return fmt.Errorf("read %s: %w", inputFile, err)
	}

	if len(entries) == 0 {
		fmt.Fprintln(a.out, "No history entries found.")
		return nil
	}

	if outputFile == "" {
		outputFile = defaultHistoryPath(a.home, targetName)
	}

	if jsonPreview {
		type entryJSON struct {
			Command   string `json:"command"`
			Timestamp int64  `json:"timestamp"`
		}
		output := struct {
			Source  string      `json:"source"`
			Target  string      `json:"target"`
			Input   string      `json:"input_file"`
			Output  string      `json:"output_file"`
			Count   int         `json:"count"`
			Entries []entryJSON `json:"entries"`
		}{
			Source: sourceName,
			Target: targetName,
			Input:  inputFile,
			Output: outputFile,
			Count:  len(entries),
		}
		for _, e := range entries {
			output.Entries = append(output.Entries, entryJSON{Command: e.command, Timestamp: e.timestamp})
		}
		encoder := json.NewEncoder(a.out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(output)
	}

	fmt.Fprintf(a.out, "Convert plan: %s -> %s\n", sourceName, targetName)
	fmt.Fprintf(a.out, "  Source: %s (%d entries)\n", inputFile, len(entries))
	fmt.Fprintf(a.out, "  Output: %s\n", outputFile)

	for _, e := range entries[:minInt(5, len(entries))] {
		fmt.Fprintf(a.out, "         %s\n", e.command)
	}
	if len(entries) > 5 {
		fmt.Fprintf(a.out, "         ... and %d more entries\n", len(entries)-5)
	}

	if !yes {
		fmt.Fprintln(a.out, "Preview only. Use --yes to write the converted history.")
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(outputFile), 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	file, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer file.Close()

	if err := writeHistory(entries, file, targetName); err != nil {
		return fmt.Errorf("write history: %w", err)
	}

	fmt.Fprintf(a.out, "Written %d entries to %s\n", len(entries), outputFile)
	a.logOperation("CONVERTED", sourceName+"->"+targetName, outputFile, "")
	return nil
}

func loadHistoryEntries(path, source string) ([]historyEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	switch source {
	case "zsh":
		return parseZshHistory(file)
	case "fish":
		return parseFishHistory(file)
	case "bash":
		return parseBashHistory(file)
	default:
		return nil, fmt.Errorf("unknown source: %s", source)
	}
}

func writeHistory(entries []historyEntry, writer io.Writer, target string) error {
	switch target {
	case "zsh":
		return writeZshHistory(entries, writer)
	case "fish":
		return writeFishHistory(entries, writer)
	case "bash":
		return writeBashHistory(entries, writer)
	default:
		return fmt.Errorf("unknown target: %s", target)
	}
}

func defaultHistoryPath(home, target string) string {
	switch target {
	case "zsh":
		return filepath.Join(home, ".zsh_history")
	case "fish":
		return filepath.Join(home, ".local", "share", "fish", "fish_history")
	case "bash":
		return filepath.Join(home, ".bash_history")
	default:
		return ""
	}
}
