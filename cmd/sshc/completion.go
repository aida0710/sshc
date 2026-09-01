package main

import (
	"fmt"
	"io"
	"strings"
)

func renderCompletion(template string) string {
	grammar := cliCompletionGrammar
	return strings.NewReplacer(
		"{{TOP_LEVEL}}", strings.Join(grammar.topLevel, " "),
		"{{HELP_TOPICS}}", strings.Join(grammar.helpTopics, " "),
		"{{SYNC_ACTIONS}}", strings.Join(grammar.syncActions, " "),
		"{{TERMINAL_ACTIONS}}", strings.Join(grammar.terminalActions, " "),
		"{{SERVICE_ACTIONS}}", strings.Join(grammar.serviceActions, " "),
		"{{VAULT_ACTIONS}}", strings.Join(grammar.vaultActions, " "),
		"{{ENCODINGS}}", strings.Join(grammar.encodings, " "),
		"{{WAIT_STATES}}", strings.Join(grammar.waitStates, " "),
		"{{SERIAL_OPTIONS}}", strings.Join(grammar.serialOptions, " "),
		"{{TELNET_OPTIONS}}", strings.Join(grammar.telnetOptions, " "),
	).Replace(template)
}

// 補完候補は生成時に設定へ焼き込まず、Tabを押した時点の `sshc ssh --list` を使う。
// Include先の追加やSync受信後にも、補完ファイルを作り直さず最新のaliasを表示するためである。
const bashCompletionTemplate = `# bash completion for sshc
_sshc_completion() {
  local current previous command candidate
  current="${COMP_WORDS[COMP_CWORD]}"
  previous=""
  if (( COMP_CWORD > 0 )); then
    previous="${COMP_WORDS[COMP_CWORD-1]}"
  fi
  command="${COMP_WORDS[1]}"

  # compgen -W は語リストの各語をシェルとして展開する。コマンド置換もそこに含ま
  # れるため、ssh_config 由来で信用できない alias を渡してはならない。静的な
  # 語彙にだけ compgen -W を使い、alias は展開せず1行ずつ読んで前方一致で絞る。
  _sshc_complete_words() {
    COMPREPLY=( $(compgen -W "$1" -- "$current") )
  }
  _sshc_complete_aliases() {
    COMPREPLY=()
    while IFS= read -r candidate; do
      [[ -z "$candidate" || "$candidate" != "$current"* ]] && continue
      COMPREPLY+=("$candidate")
    done < <(command sshc ssh --list 2>/dev/null)
    if [[ -n "$1" ]]; then
      COMPREPLY+=( $(compgen -W "$1" -- "$current") )
    fi
  }

  if (( COMP_CWORD == 1 )); then
    _sshc_complete_words "{{TOP_LEVEL}}"
    return
  fi

  case "$previous" in
    --encoding) _sshc_complete_words "{{ENCODINGS}}"; return ;;
    --data-bits) _sshc_complete_words "5 6 7 8"; return ;;
    --parity) _sshc_complete_words "none odd even mark space"; return ;;
    --stop-bits) _sshc_complete_words "1 1.5 2"; return ;;
    --flow) _sshc_complete_words "none rtscts xonxoff"; return ;;
    --dtr|--rts) _sshc_complete_words "on off"; return ;;
    --line-ending) _sshc_complete_words "none cr lf crlf"; return ;;
    --for) _sshc_complete_words "{{WAIT_STATES}}"; return ;;
    --script)
      COMPREPLY=()
      while IFS= read -r candidate; do COMPREPLY+=("$candidate"); done < <(compgen -f -- "$current")
      return
      ;;
  esac

  case "$command" in
    engine)
      _sshc_complete_words "--port --replace --help"
      ;;
    completion)
      if (( COMP_CWORD == 2 )); then _sshc_complete_words "bash zsh fish --help"; fi
      ;;
    ssh)
      if (( COMP_CWORD == 2 )); then
        _sshc_complete_aliases "--list --help"
      elif (( COMP_CWORD == 3 )) && [[ "${COMP_WORDS[2]}" != -* ]]; then
        _sshc_complete_words "--non-interactive"
      elif [[ "$previous" == "--non-interactive" ]]; then
        _sshc_complete_words "--"
      fi
      ;;
    info)
      if (( COMP_CWORD == 2 )); then _sshc_complete_aliases; else _sshc_complete_words "--json"; fi
      ;;
    sync)
      if (( COMP_CWORD == 2 )); then
        _sshc_complete_words "{{SYNC_ACTIONS}} --json --help"
      else
        case "${COMP_WORDS[2]}" in
          push|pull) _sshc_complete_words "--force --json --help" ;;
          now) _sshc_complete_words "--json --help" ;;
          auto)
            if (( COMP_CWORD == 3 )); then _sshc_complete_words "on off --help"; else _sshc_complete_words "--json"; fi
            ;;
        esac
      fi
      ;;
    terminal)
      if (( COMP_CWORD == 2 )); then
        _sshc_complete_words "{{TERMINAL_ACTIONS}} --help"
      elif [[ "${COMP_WORDS[2]}" == "create" && "$COMP_CWORD" -eq 3 ]]; then
        _sshc_complete_words "shell ssh --help"
      elif [[ "${COMP_WORDS[2]}" == "create" && "${COMP_WORDS[3]}" == "ssh" && "$COMP_CWORD" -eq 4 ]]; then
        _sshc_complete_aliases
      elif (( COMP_CWORD == 3 )); then
        _sshc_complete_words "--help"
      else
        case "${COMP_WORDS[2]}" in
          list) _sshc_complete_words "--json --help" ;;
          show|close) if (( COMP_CWORD >= 4 )); then _sshc_complete_words "--json"; fi ;;
          create) if [[ "${COMP_WORDS[3]}" == "shell" && COMP_CWORD -ge 4 || "${COMP_WORDS[3]}" == "ssh" && COMP_CWORD -ge 5 ]]; then _sshc_complete_words "--json"; fi ;;
          rename) if (( COMP_CWORD >= 5 )); then _sshc_complete_words "--json"; fi ;;
          read) if (( COMP_CWORD >= 4 )); then _sshc_complete_words "--cursor --limit --json"; fi ;;
          send) if (( COMP_CWORD >= 4 )); then _sshc_complete_words "--text --no-enter --json"; fi ;;
          wait) if (( COMP_CWORD >= 4 )); then _sshc_complete_words "--for --timeout --json"; fi ;;
        esac
      fi
      ;;
    serial)
      _sshc_complete_words "{{SERIAL_OPTIONS}}"
      ;;
    telnet)
      _sshc_complete_words "{{TELNET_OPTIONS}}"
      ;;
    status)
      _sshc_complete_words "--json --help"
      ;;
    service)
      if (( COMP_CWORD == 2 )); then _sshc_complete_words "{{SERVICE_ACTIONS}} --help"; elif (( COMP_CWORD == 3 )); then _sshc_complete_words "--help"; fi
      ;;
    vault)
      if (( COMP_CWORD == 2 )); then _sshc_complete_words "{{VAULT_ACTIONS}} --help"; elif (( COMP_CWORD == 3 )); then _sshc_complete_words "--help"; fi
      ;;
    help)
      if (( COMP_CWORD == 2 )); then
        _sshc_complete_words "{{HELP_TOPICS}}"
      elif [[ "${COMP_WORDS[2]}" == "sync" ]]; then
        _sshc_complete_words "{{SYNC_ACTIONS}}"
      elif [[ "${COMP_WORDS[2]}" == "terminal" ]]; then
        _sshc_complete_words "{{TERMINAL_ACTIONS}}"
      elif [[ "${COMP_WORDS[2]}" == "service" ]]; then
        _sshc_complete_words "{{SERVICE_ACTIONS}}"
      elif [[ "${COMP_WORDS[2]}" == "vault" ]]; then
        _sshc_complete_words "{{VAULT_ACTIONS}}"
      fi
      ;;
  esac
}
complete -F _sshc_completion sshc
`

const zshCompletionTemplate = `#compdef sshc
# zsh completion for sshc
_sshc() {
  local command previous
  local -a aliases candidates
  command="${words[2]}"
  previous="${words[CURRENT-1]}"

  _sshc_aliases() {
    aliases=("${(@f)$(command sshc ssh --list 2>/dev/null)}")
    _describe 'SSH Host alias' aliases
  }
  _sshc_values() {
    candidates=("${(@s: :)1}")
    _describe 'value' candidates
  }

  if (( CURRENT == 2 )); then
    _sshc_values '{{TOP_LEVEL}}'
    return
  fi

  case "$previous" in
    --encoding) _sshc_values '{{ENCODINGS}}'; return ;;
    --data-bits) _sshc_values '5 6 7 8'; return ;;
    --parity) _sshc_values 'none odd even mark space'; return ;;
    --stop-bits) _sshc_values '1 1.5 2'; return ;;
    --flow) _sshc_values 'none rtscts xonxoff'; return ;;
    --dtr|--rts) _sshc_values 'on off'; return ;;
    --line-ending) _sshc_values 'none cr lf crlf'; return ;;
    --for) _sshc_values '{{WAIT_STATES}}'; return ;;
    --script) _files; return ;;
  esac

  case "$command" in
    engine) _sshc_values '--port --replace --help' ;;
    completion) (( CURRENT == 3 )) && _sshc_values 'bash zsh fish --help' ;;
    ssh)
      if (( CURRENT == 3 )); then
        aliases=("${(@f)$(command sshc ssh --list 2>/dev/null)}" --list --help)
        _describe 'SSH Host alias or option' aliases
      elif (( CURRENT == 4 )) && [[ "${words[3]}" != -* ]]; then
        _sshc_values '--non-interactive'
      elif [[ "$previous" == '--non-interactive' ]]; then
        _sshc_values '--'
      fi
      ;;
    info)
      if (( CURRENT == 3 )); then _sshc_aliases; else _sshc_values '--json'; fi
      ;;
    sync)
      if (( CURRENT == 3 )); then
        _sshc_values '{{SYNC_ACTIONS}} --json --help'
      else
        case "${words[3]}" in
          push|pull) _sshc_values '--force --json --help' ;;
          now) _sshc_values '--json --help' ;;
          auto) if (( CURRENT == 4 )); then _sshc_values 'on off --help'; else _sshc_values '--json'; fi ;;
        esac
      fi
      ;;
    terminal)
      if (( CURRENT == 3 )); then
        _sshc_values '{{TERMINAL_ACTIONS}} --help'
      elif [[ "${words[3]}" == 'create' && "$CURRENT" -eq 4 ]]; then
        _sshc_values 'shell ssh --help'
      elif [[ "${words[3]}" == 'create' && "${words[4]}" == 'ssh' && "$CURRENT" -eq 5 ]]; then
        _sshc_aliases
      elif (( CURRENT == 4 )); then
        _sshc_values '--help'
      else
        case "${words[3]}" in
          list) _sshc_values '--json --help' ;;
          show|close) (( CURRENT >= 5 )) && _sshc_values '--json' ;;
          create) if [[ "${words[4]}" == 'shell' && CURRENT -ge 5 || "${words[4]}" == 'ssh' && CURRENT -ge 6 ]]; then _sshc_values '--json'; fi ;;
          rename) (( CURRENT >= 6 )) && _sshc_values '--json' ;;
          read) (( CURRENT >= 5 )) && _sshc_values '--cursor --limit --json' ;;
          send) (( CURRENT >= 5 )) && _sshc_values '--text --no-enter --json' ;;
          wait) (( CURRENT >= 5 )) && _sshc_values '--for --timeout --json' ;;
        esac
      fi
      ;;
    serial) _sshc_values '{{SERIAL_OPTIONS}}' ;;
    telnet) _sshc_values '{{TELNET_OPTIONS}}' ;;
    status) _sshc_values '--json --help' ;;
    service) if (( CURRENT == 3 )); then _sshc_values '{{SERVICE_ACTIONS}} --help'; elif (( CURRENT == 4 )); then _sshc_values '--help'; fi ;;
    vault) if (( CURRENT == 3 )); then _sshc_values '{{VAULT_ACTIONS}} --help'; elif (( CURRENT == 4 )); then _sshc_values '--help'; fi ;;
    help)
      if (( CURRENT == 3 )); then
        _sshc_values '{{HELP_TOPICS}}'
      else
        case "${words[3]}" in
          sync) _sshc_values '{{SYNC_ACTIONS}}' ;;
          terminal) _sshc_values '{{TERMINAL_ACTIONS}}' ;;
          service) _sshc_values '{{SERVICE_ACTIONS}}' ;;
          vault) _sshc_values '{{VAULT_ACTIONS}}' ;;
        esac
      fi
      ;;
  esac
}
compdef _sshc sshc
`

const fishCompletionTemplate = `# fish completion for sshc
function __sshc_prefix
    set -l words (commandline -opc)
    set -e words[1]
    if test (count $words) -ne (count $argv)
        return 1
    end
    for index in (seq (count $argv))
        if test "$words[$index]" != "$argv[$index]"
            return 1
        end
    end
    return 0
end

function __sshc_command
    set -l words (commandline -opc)
    test (count $words) -ge 2; and test "$words[2]" = "$argv[1]"
end

function __sshc_action
    set -l words (commandline -opc)
    test (count $words) -ge 3; and test "$words[2]" = "$argv[1]"; and test "$words[3]" = "$argv[2]"
end

function __sshc_previous
    set -l words (commandline -opc)
    test (count $words) -ge 2; and test "$words[-1]" = "$argv[1]"
end

function __sshc_min_words
    set -l words (commandline -opc)
    test (count $words) -ge "$argv[1]"
end

function __sshc_sync_json
    if __sshc_prefix sync
        return 0
    end
    if __sshc_action sync push; or __sshc_action sync pull; or __sshc_action sync now
        return 0
    end
    __sshc_action sync auto; and __sshc_min_words 4
end

function __sshc_terminal_options
    set -l words (commandline -opc)
    if test (count $words) -lt 3; or test "$words[2]" != terminal
        return 1
    end
    switch "$words[3]"
        case list
            return 0
        case show close read send wait
            test (count $words) -ge 4
        case create
            if test (count $words) -ge 4; and test "$words[4]" = shell
                return 0
            end
            test (count $words) -ge 5; and test "$words[4]" = ssh
        case rename
            test (count $words) -ge 5
        case '*'
            return 1
    end
end

complete -c sshc -f -n '__sshc_prefix' -a '{{TOP_LEVEL}}'
complete -c sshc -f -n '__sshc_prefix completion' -a 'bash zsh fish --help'
complete -c sshc -f -n '__sshc_prefix ssh' -a '(command sshc ssh --list 2>/dev/null)'
complete -c sshc -f -n '__sshc_prefix ssh' -l list -d 'Print every concrete Host alias'
complete -c sshc -f -n '__sshc_prefix ssh' -l help -d 'Show help for sshc ssh'
complete -c sshc -f -n '__sshc_command ssh; and __sshc_min_words 3; and not contains -- --non-interactive (commandline -opc)' -a '--non-interactive'
complete -c sshc -f -n '__sshc_previous --non-interactive' -a '--'
complete -c sshc -f -n '__sshc_prefix info' -a '(command sshc ssh --list 2>/dev/null)'
complete -c sshc -f -n '__sshc_prefix terminal create' -a 'shell ssh'
complete -c sshc -f -n '__sshc_prefix terminal create ssh' -a '(command sshc ssh --list 2>/dev/null)'
complete -c sshc -f -n '__sshc_prefix terminal list; or __sshc_prefix terminal show; or __sshc_prefix terminal read; or __sshc_prefix terminal send; or __sshc_prefix terminal wait; or __sshc_prefix terminal rename; or __sshc_prefix terminal close' -a '--help'

complete -c sshc -f -n '__sshc_prefix sync' -a '{{SYNC_ACTIONS}}'
complete -c sshc -f -n '__sshc_prefix sync auto' -a 'on off'
complete -c sshc -f -n '__sshc_prefix sync setup; or __sshc_prefix sync push; or __sshc_prefix sync pull; or __sshc_prefix sync now; or __sshc_prefix sync auto' -a '--help'
complete -c sshc -f -n '__sshc_prefix terminal' -a '{{TERMINAL_ACTIONS}}'
complete -c sshc -f -n '__sshc_prefix service' -a '{{SERVICE_ACTIONS}} --help'
complete -c sshc -f -n '__sshc_prefix vault' -a '{{VAULT_ACTIONS}} --help'
complete -c sshc -f -n '__sshc_prefix service install; or __sshc_prefix service status; or __sshc_prefix service disable' -a '--help'
complete -c sshc -f -n '__sshc_prefix vault status; or __sshc_prefix vault create; or __sshc_prefix vault unlock; or __sshc_prefix vault lock; or __sshc_prefix vault change-password' -a '--help'
complete -c sshc -f -n '__sshc_prefix help' -a '{{HELP_TOPICS}}'
complete -c sshc -f -n '__sshc_prefix help sync' -a '{{SYNC_ACTIONS}}'
complete -c sshc -f -n '__sshc_prefix help terminal' -a '{{TERMINAL_ACTIONS}}'
complete -c sshc -f -n '__sshc_prefix help service' -a '{{SERVICE_ACTIONS}}'
complete -c sshc -f -n '__sshc_prefix help vault' -a '{{VAULT_ACTIONS}}'

complete -c sshc -f -n '__sshc_command engine' -a '--port --replace --help'
complete -c sshc -f -n '__sshc_command info; and __sshc_min_words 3' -a '--json'
complete -c sshc -f -n '__sshc_command status' -a '--json --help'
complete -c sshc -f -n '__sshc_action sync push; or __sshc_action sync pull' -a '--force'
complete -c sshc -f -n '__sshc_sync_json' -a '--json'
complete -c sshc -f -n '__sshc_terminal_options' -a '--json'
complete -c sshc -f -n '__sshc_action terminal read; and __sshc_min_words 4' -a '--cursor --limit'
complete -c sshc -f -n '__sshc_action terminal send; and __sshc_min_words 4' -a '--text --no-enter'
complete -c sshc -f -n '__sshc_action terminal wait; and __sshc_min_words 4' -a '--for --timeout'
complete -c sshc -f -n '__sshc_previous --for' -a '{{WAIT_STATES}}'

complete -c sshc -f -n '__sshc_command serial' -a '{{SERIAL_OPTIONS}}'
complete -c sshc -f -n '__sshc_command telnet' -a '{{TELNET_OPTIONS}}'
complete -c sshc -f -n '__sshc_previous --encoding' -a '{{ENCODINGS}}'
complete -c sshc -f -n '__sshc_previous --data-bits' -a '5 6 7 8'
complete -c sshc -f -n '__sshc_previous --parity' -a 'none odd even mark space'
complete -c sshc -f -n '__sshc_previous --stop-bits' -a '1 1.5 2'
complete -c sshc -f -n '__sshc_previous --flow' -a 'none rtscts xonxoff'
complete -c sshc -f -n '__sshc_previous --dtr; or __sshc_previous --rts' -a 'on off'
complete -c sshc -f -n '__sshc_previous --line-ending' -a 'none cr lf crlf'
complete -c sshc -F -n '__sshc_previous --script'
`

var (
	bashCompletion = renderCompletion(bashCompletionTemplate)
	zshCompletion  = renderCompletion(zshCompletionTemplate)
	fishCompletion = renderCompletion(fishCompletionTemplate)
)

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
