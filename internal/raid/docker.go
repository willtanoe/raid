package raid

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func (a *app) runDocker(args []string) error {
	flags, rest, err := parseCommonFlags(args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return errors.New("usage: raid docker [--dry-run] [--yes]")
	}
	if flags.permanent {
		return errors.New("--permanent applies only to file cleanup commands")
	}
	if !commandExists("docker") {
		return errors.New("docker is not installed")
	}

	dfCmd := exec.Command("docker", "system", "df")
	dfCmd.Stdout = a.out
	dfCmd.Stderr = a.errOut
	_ = dfCmd.Run()

	actions := []struct {
		name string
		args []string
	}{
		{"Remove stopped containers", []string{"docker", "container", "prune", "-f"}},
		{"Remove unused images", []string{"docker", "image", "prune", "-f"}},
		{"Remove unused volumes", []string{"docker", "volume", "prune", "-f"}},
		{"Remove build cache", []string{"docker", "builder", "prune", "-f"}},
	}

	fmt.Fprintln(a.out, "\nDocker cleanup plan:")
	for _, action := range actions {
		fmt.Fprintf(a.out, "PLAN     %s: %s\n", action.name, strings.Join(action.args, " "))
	}

	if !flags.yes {
		fmt.Fprintln(a.out, "Preview only. Use --yes to execute all Docker cleanup actions.")
		return nil
	}

	var failures []error
	for _, action := range actions {
		if err := a.runExternal("docker", action.args); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", action.name, err))
		}
	}
	return errors.Join(failures...)
}
