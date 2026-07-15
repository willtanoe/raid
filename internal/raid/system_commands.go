package raid

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"strings"
)

var packageNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9+._:@/-]*$`)

type packageMatch struct {
	manager string
	name    string
	remove  []string
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func commandSucceeds(name string, args ...string) bool {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

func (a *app) runExternal(action string, command []string) error {
	if len(command) == 0 {
		return errors.New("empty external command")
	}
	fmt.Fprintf(a.out, "RUN      %s\n", strings.Join(command, " "))
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = a.out
	cmd.Stderr = a.errOut
	err := cmd.Run()
	if err != nil {
		a.logOperation("FAILED", action, strings.Join(command, " "), err.Error())
		return err
	}
	a.logOperation("EXECUTED", action, strings.Join(command, " "), "")
	return nil
}

func detectPackage(name string) []packageMatch {
	var matches []packageMatch
	if commandExists("dpkg-query") {
		output, err := exec.Command("dpkg-query", "-W", "-f=${Status}", name).Output()
		if err == nil && strings.TrimSpace(string(output)) == "install ok installed" {
			matches = append(matches, packageMatch{manager: "apt", name: name, remove: []string{"sudo", "-n", "apt-get", "remove", "--", name}})
		}
	}
	if commandExists("rpm") {
		output, err := exec.Command("rpm", "-q", "--qf=%{VERSION}", name).Output()
		if err == nil && strings.TrimSpace(string(output)) != "" {
			matches = append(matches, packageMatch{manager: "dnf", name: name, remove: []string{"sudo", "-n", "dnf", "remove", "-y", name}})
		}
	}
	if commandExists("pacman") && commandSucceeds("pacman", "-Q", name) {
		matches = append(matches, packageMatch{manager: "pacman", name: name, remove: []string{"sudo", "-n", "pacman", "-R", "--noconfirm", name}})
	}
	if commandExists("snap") && commandSucceeds("snap", "list", name) {
		matches = append(matches, packageMatch{manager: "snap", name: name, remove: []string{"sudo", "-n", "snap", "remove", name}})
	}
	if commandExists("flatpak") {
		output, err := exec.Command("flatpak", "list", "--app", "--columns=application").Output()
		if err == nil {
			for _, installed := range strings.Fields(string(output)) {
				if installed == name {
					matches = append(matches, packageMatch{manager: "flatpak", name: installed, remove: []string{"flatpak", "uninstall", "--noninteractive", installed}})
					break
				}
			}
		}
	}
	return matches
}

func (a *app) runUninstall(args []string) error {
	flags, rest, err := parseCommonFlags(args)
	if err != nil {
		return err
	}
	if err := requireArgs(rest, "raid uninstall <package> [--dry-run] [--yes]"); err != nil {
		return err
	}
	if flags.permanent {
		return errors.New("--permanent applies only to file cleanup commands")
	}
	if len(rest) != 1 || !packageNamePattern.MatchString(rest[0]) {
		return errors.New("provide one exact APT, DNF, Pacman, Snap, or Flatpak package name")
	}
	matches := detectPackage(rest[0])
	if len(matches) == 0 {
		return fmt.Errorf("package %q was not found in any known package manager", rest[0])
	}
	if len(matches) > 1 {
		managers := make([]string, 0, len(matches))
		for _, match := range matches {
			managers = append(managers, match.manager)
		}
		return fmt.Errorf("package exists in multiple managers (%s); specify the manager: raid uninstall %s/<package> or use the exact package name", strings.Join(managers, ", "), managers[0])
	}
	match := matches[0]
	fmt.Fprintf(a.out, "Uninstall plan: %s package %s\n", match.manager, match.name)
	fmt.Fprintf(a.out, "Command: %s\n", strings.Join(match.remove, " "))
	if !flags.yes {
		fmt.Fprintln(a.out, "Preview only. Use --yes after reviewing the exact package.")
		return nil
	}
	return a.runExternal("uninstall", match.remove)
}

type maintenanceAction struct {
	label   string
	command []string
	needs   string
}

func (a *app) runOptimize(args []string) error {
	flags, rest, err := parseCommonFlags(args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return errors.New("usage: raid optimize [--dry-run] [--yes]")
	}
	if flags.permanent {
		return errors.New("--permanent applies only to file cleanup commands")
	}
	actions := []maintenanceAction{
		{label: "Reload user systemd units", command: []string{"systemctl", "--user", "daemon-reload"}, needs: "systemctl"},
		{label: "Rebuild font cache", command: []string{"fc-cache", "-f"}, needs: "fc-cache"},
		{label: "Vacuum journal entries older than 14 days", command: []string{"sudo", "-n", "journalctl", "--vacuum-time=14d"}, needs: "journalctl"},
		{label: "Clean downloaded APT package files", command: []string{"sudo", "-n", "apt-get", "clean"}, needs: "apt-get"},
		{label: "Remove unused APT packages and old kernels", command: []string{"sudo", "-n", "apt-get", "autoremove", "-y"}, needs: "apt-get"},
		{label: "Clean DNF package cache", command: []string{"sudo", "-n", "dnf", "clean", "all"}, needs: "dnf"},
		{label: "Remove unused DNF packages", command: []string{"sudo", "-n", "dnf", "autoremove", "-y"}, needs: "dnf"},
		{label: "Clean Pacman package cache", command: []string{"sudo", "-n", "pacman", "-Sc", "--noconfirm"}, needs: "pacman"},
		{label: "Refresh Snap packages", command: []string{"snap", "refresh"}, needs: "snap"},
	}
	var available []maintenanceAction
	for _, action := range actions {
		if commandExists(action.needs) {
			available = append(available, action)
			fmt.Fprintf(a.out, "PLAN     %s\n", action.label)
			fmt.Fprintf(a.out, "         %s\n", strings.Join(action.command, " "))
		}
	}
	if !flags.yes {
		fmt.Fprintln(a.out, "Preview only. Use --yes to execute available tasks.")
		return nil
	}
	var failures []error
	for _, action := range available {
		if err := a.runExternal("optimize", action.command); err != nil {
			fmt.Fprintf(a.errOut, "task failed: %s: %v\n", action.label, err)
			failures = append(failures, fmt.Errorf("%s: %w", action.label, err))
		}
	}
	return errors.Join(failures...)
}

func (a *app) runFingerprint(args []string) error {
	mode := "status"
	yes := false
	var rest []string
	for _, arg := range args {
		if arg == "--yes" || arg == "-y" {
			yes = true
		} else {
			rest = append(rest, arg)
		}
	}
	if len(rest) > 1 {
		return errors.New("usage: raid fingerprint [status|enroll] [--yes]")
	}
	if len(rest) == 1 {
		mode = rest[0]
	}
	if !commandExists("fprintd-list") && !commandExists("fprintd-enroll") {
		return errors.New("fprintd is not installed; install the fprintd package first")
	}
	currentUser, err := user.Current()
	if err != nil || currentUser.Username == "" {
		return errors.New("could not determine the current user")
	}
	username := currentUser.Username
	switch mode {
	case "status":
		if !commandExists("fprintd-list") {
			return errors.New("fprintd-list is unavailable")
		}
		return a.runExternal("fingerprint-status", []string{"fprintd-list", username})
	case "enroll":
		if !commandExists("fprintd-enroll") {
			return errors.New("fprintd-enroll is unavailable")
		}
		if !yes {
			fmt.Fprintf(a.out, "PLAN     Enroll a fingerprint for user %s\n", username)
			fmt.Fprintln(a.out, "Preview only. Use --yes to start interactive enrollment.")
			return nil
		}
		return a.runExternal("fingerprint-enroll", []string{"fprintd-enroll", username})
	default:
		return errors.New("usage: raid fingerprint [status|enroll]")
	}
}

func (a *app) runUpdate(args []string) error {
	flags, rest, err := parseCommonFlags(args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return errors.New("usage: raid update [--dry-run] [--yes]")
	}
	if flags.permanent {
		return errors.New("--permanent applies only to file cleanup commands")
	}

	actions := []maintenanceAction{
		{label: "Update APT package lists", command: []string{"sudo", "-n", "apt-get", "update"}, needs: "apt-get"},
		{label: "Upgrade APT packages", command: []string{"sudo", "-n", "apt-get", "upgrade", "-y"}, needs: "apt-get"},
		{label: "Upgrade DNF packages", command: []string{"sudo", "-n", "dnf", "upgrade", "-y"}, needs: "dnf"},
		{label: "Upgrade Pacman packages", command: []string{"sudo", "-n", "pacman", "-Syu", "--noconfirm"}, needs: "pacman"},
		{label: "Refresh Snap packages", command: []string{"snap", "refresh"}, needs: "snap"},
	}

	if commandExists("flatpak") {
		actions = append(actions, maintenanceAction{
			label: "Update Flatpak packages", command: []string{"flatpak", "update", "-y"}, needs: "flatpak",
		})
	}

	var available []maintenanceAction
	for _, action := range actions {
		if commandExists(action.needs) {
			available = append(available, action)
			fmt.Fprintf(a.out, "PLAN     %s\n", action.label)
			fmt.Fprintf(a.out, "         %s\n", strings.Join(action.command, " "))
		}
	}

	if !flags.yes {
		if len(available) == 0 {
			fmt.Fprintln(a.out, "No package managers found.")
			return nil
		}
		fmt.Fprintln(a.out, "Preview only. Use --yes to run all updates.")
		return nil
	}

	var failures []error
	for _, action := range available {
		if err := a.runExternal("update", action.command); err != nil {
			fmt.Fprintf(a.errOut, "task failed: %s: %v\n", action.label, err)
			failures = append(failures, fmt.Errorf("%s: %w", action.label, err))
		}
	}
	return errors.Join(failures...)
}
