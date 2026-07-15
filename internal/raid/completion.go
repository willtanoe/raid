package raid

import (
	"errors"
	"fmt"
)

var raidCommands = "clean uninstall optimize analyze status purge installer update docker search convert history completion fingerprint version help"

func (a *app) runCompletion(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: raid completion <bash|zsh|fish>")
	}
	switch args[0] {
	case "bash":
		fmt.Fprintf(a.out, `_raid_completion() {
  local current="${COMP_WORDS[COMP_CWORD]}"
  if [[ $COMP_CWORD -eq 1 ]]; then
    COMPREPLY=( $(compgen -W %q -- "$current") )
  else
    COMPREPLY=( $(compgen -W "--dry-run --yes --permanent --json --text" -- "$current") )
  fi
}
complete -F _raid_completion raid
`, raidCommands)
	case "zsh":
		fmt.Fprintf(a.out, `#compdef raid
_raid() {
  local -a commands
  commands=(%s)
  if (( CURRENT == 2 )); then
    _describe 'command' commands
  else
    _arguments '--dry-run[preview only]' '--yes[execute plan]' '--permanent[skip Trash]' '--json[JSON output]' '--text[text output]'
  fi
}
_raid
`, raidCommands)
	case "fish":
		fmt.Fprintf(a.out, "complete -c raid -f\n")
		for _, command := range []string{"clean", "uninstall", "optimize", "analyze", "status", "purge", "installer", "update", "docker", "search", "convert", "history", "completion", "fingerprint", "version", "help"} {
			fmt.Fprintf(a.out, "complete -c raid -n '__fish_use_subcommand' -a %s\n", command)
		}
	default:
		return errors.New("supported shells: bash, zsh, fish")
	}
	return nil
}
