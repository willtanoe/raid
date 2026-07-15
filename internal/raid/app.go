package raid

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type app struct {
	home     string
	stateDir string
	config   config
	out      io.Writer
	errOut   io.Writer
}

// Version is replaced through ldflags in release builds.
var Version = "0.2.1"

// Run executes Raid with the provided command-line arguments.
func Run(args []string) error {
	application, err := newApp()
	if err != nil {
		return err
	}
	return application.run(args)
}

func newApp() (*app, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	if override := os.Getenv("RAID_HOME"); override != "" && os.Getenv("RAID_TEST_MODE") == "1" {
		home, err = filepath.Abs(override)
		if err != nil {
			return nil, err
		}
	}
	resolvedHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return nil, fmt.Errorf("resolve home: %w", err)
	}
	home = resolvedHome
	stateDir := ""
	if os.Getenv("RAID_TEST_MODE") == "1" {
		stateDir = os.Getenv("RAID_STATE_DIR")
	}
	if stateDir == "" {
		stateDir = filepath.Join(home, ".local", "state", "raid")
	}
	return &app{home: filepath.Clean(home), stateDir: stateDir, config: loadConfig(home), out: os.Stdout, errOut: os.Stderr}, nil
}

func (a *app) run(args []string) error {
	if len(args) == 0 {
		if isInteractiveTerminal() {
			return a.runInteractive()
		}
		a.printHelp()
		return nil
	}

	command := args[0]
	args = args[1:]
	switch command {
	case "help", "-h", "--help":
		a.printHelp()
		return nil
	case "version", "-v", "--version":
		fmt.Fprintf(a.out, "Raid %s\n", Version)
		return nil
	case "clean":
		return a.runClean(args)
	case "uninstall":
		return a.runUninstall(args)
	case "optimize", "optimise":
		return a.runOptimize(args)
	case "analyze", "analyse":
		return a.runAnalyze(args)
	case "status":
		return a.runStatus(args)
	case "purge":
		return a.runPurge(args)
	case "installer":
		return a.runInstaller(args)
	case "history":
		return a.runHistory(args)
	case "completion":
		return a.runCompletion(args)
	case "fingerprint", "touchid":
		return a.runFingerprint(args)
	case "update":
		return a.runUpdate(args)
	case "docker":
		return a.runDocker(args)
	case "search":
		return a.runSearch(args)
	case "convert":
		return a.runConvert(args)
	default:
		return fmt.Errorf("unknown command %q; run 'raid help'", command)
	}
}

func (a *app) runInteractive() error {
	for {
		selected, err := a.runMainTUI()
		if err != nil {
			return err
		}
		if len(selected) == 0 {
			return nil
		}

		err = a.run(selected)
		if err != nil {
			fmt.Fprintln(a.errOut, "raid:", err)
		}
		if !isFullScreenCommand(selected[0]) || err != nil {
			fmt.Fprint(a.out, "\nPress Enter to return to the menu...")
			_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		}
	}
}

func isFullScreenCommand(command string) bool {
	switch command {
	case "analyze", "analyse", "status":
		return true
	default:
		return false
	}
}

func (a *app) printHelp() {
	fmt.Fprintln(a.out, `Raid - safe Linux cleanup and maintenance

Usage:
  raid <command> [options]

Commands:
  clean        Clean conservative user and developer caches
  uninstall    Uninstall an APT, DNF, Pacman, Snap, or Flatpak package
  optimize     Run bounded Linux maintenance tasks (systemd, fonts, journals, package caches)
  analyze      Analyze disk usage for a directory
  status       Show a compact system health snapshot
  purge        Remove rebuildable project artifacts
  installer    Find or remove old installer files
  update       Check and apply system updates (APT, DNF, Pacman, Snap, Flatpak)
  docker       Clean unused Docker containers, images, and volumes
  search       Find files by size, age, or name pattern
  convert      Convert shell history between zsh, fish, and bash formats
  history      Show Raid operation history
  completion   Generate shell completion
  fingerprint  Inspect or enroll Linux fingerprint support
  version      Show the installed version

Destructive commands preview by default. Use --yes to execute and
--dry-run to force preview. File removals use Trash unless --permanent
is explicitly supplied.`)
}

type commonFlags struct {
	dryRun    bool
	yes       bool
	permanent bool
}

func parseCommonFlags(args []string) (commonFlags, []string, error) {
	var flags commonFlags
	var rest []string
	for _, arg := range args {
		switch arg {
		case "--dry-run", "-n":
			flags.dryRun = true
		case "--yes", "-y":
			flags.yes = true
		case "--permanent":
			flags.permanent = true
		default:
			if strings.HasPrefix(arg, "-") {
				return flags, nil, fmt.Errorf("unknown option %q", arg)
			}
			rest = append(rest, arg)
		}
	}
	if flags.dryRun {
		flags.yes = false
	}
	return flags, rest, nil
}

func requireArgs(args []string, usage string) error {
	if len(args) == 0 {
		return errors.New("usage: " + usage)
	}
	return nil
}
