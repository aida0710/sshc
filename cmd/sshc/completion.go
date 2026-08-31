package main

import (
	"fmt"
	"io"
)

// 補完候補は生成時に設定へ焼き込まず、Tabを押した時点の `sshc ssh --list` を使う。
// Include先の追加やSync受信後にも、補完ファイルを作り直さず最新のaliasを表示するためである。
const bashCompletion = `# bash completion for sshc
_sshc_completion() {
  local current aliases
  current="${COMP_WORDS[COMP_CWORD]}"
  if (( COMP_CWORD == 1 )); then
    COMPREPLY=( $(compgen -W "engine ssh info sync terminal serial telnet open status update service vault version help completion" -- "$current") )
    return
  fi
  if [[ "${COMP_WORDS[1]}" == "ssh" && "$COMP_CWORD" -eq 2 ]]; then
    aliases="$(command sshc ssh --list 2>/dev/null)"
    COMPREPLY=( $(compgen -W "$aliases --list --help" -- "$current") )
  fi
}
complete -F _sshc_completion sshc
`

const zshCompletion = `#compdef sshc
# zsh completion for sshc
_sshc() {
  if (( CURRENT == 2 )); then
    local -a commands
    commands=(engine ssh info sync terminal serial telnet open status update service vault version help completion)
    _describe 'sshc command' commands
    return
  fi
  if (( CURRENT == 3 )) && [[ "${words[2]}" == "ssh" ]]; then
    local -a aliases options
    aliases=("${(@f)$(command sshc ssh --list 2>/dev/null)}")
    options=(--list --help)
    _describe 'SSH Host alias' aliases
    _describe 'option' options
  fi
}
compdef _sshc sshc
`

const fishCompletion = `# fish completion for sshc
function __sshc_needs_command
    set -l words (commandline -opc)
    test (count $words) -eq 1
end

function __sshc_ssh_needs_alias
    set -l words (commandline -opc)
    test (count $words) -eq 2; and test "$words[2]" = ssh
end

complete -c sshc -f -n __sshc_needs_command -a 'engine ssh info sync terminal serial telnet open status update service vault version help completion'
complete -c sshc -f -n __sshc_ssh_needs_alias -a '(command sshc ssh --list 2>/dev/null)'
complete -c sshc -f -n __sshc_ssh_needs_alias -l list -d 'Print every concrete Host alias'
complete -c sshc -f -n __sshc_ssh_needs_alias -l help -d 'Show help for sshc ssh'
`

func writeCompletion(output io.Writer, shell string) error {
	var script string
	switch shell {
	case "bash":
		script = bashCompletion
	case "zsh":
		script = zshCompletion
	case "fish":
		script = fishCompletion
	default:
		return fmt.Errorf("unsupported completion shell %q", shell)
	}
	_, err := io.WriteString(output, script)
	return err
}
