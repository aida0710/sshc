package main

import (
	"fmt"
	"strconv"
	"strings"
)

// commandOption は、コマンドが受け取るオプション 1 つである。値は name で引く。
type commandOption struct {
	name string
	// aliases は、同じオプションの別名（--jobs に対する -j など）。
	aliases    []string
	takesValue bool
}

func switchOption(name string, aliases ...string) commandOption {
	return commandOption{name: name, aliases: aliases}
}

func valueOption(name string, aliases ...string) commandOption {
	return commandOption{name: name, aliases: aliases, takesValue: true}
}

// spellings は、断るときの文に出す書き方である（"-j or --jobs"）。
func (option commandOption) spellings() string {
	return strings.Join(append(append([]string(nil), option.aliases...), option.name), " or ")
}

// jsonOption と yesOption は、多くのコマンドが同じ意味で受け取るオプションである。
var (
	jsonOption = switchOption("--json")
	yesOption  = switchOption("--yes", "-y")
)

// commandOptions は、1 つのコマンドが受け取るオプションの一覧である。
//
// どのコマンドのオプションも parse の同じ規則で読む。値を取る長い名前のオプションは
// --name value と --name=value の両方を受け、同じオプションを別名も含めて 2 回指定したら
// 断る。規則をコマンドごとに書くと、= の形を受けるかや 2 回目の断り方がずれるためである。
type commandOptions struct {
	// command は、断るときの文の先頭に付けるコマンド名（"sftp"、"terminal read"）。
	command string
	options []commandOption
	// endsAtDelimiter が true なら、"--" より後ろをオプションとして読まずに rest へ渡す。
	endsAtDelimiter bool
}

// commandArguments は、commandOptions の parse で読んだ引数である。
type commandArguments struct {
	command string
	// values は、指定されたオプションの値を name で引く。値を取らないオプションは空文字列になる。
	values      map[string]string
	positionals []string
	// delimited は "--" があったかどうか、rest はその後ろの引数である。
	delimited bool
	rest      []string
}

// integerBounds は、整数の値を取るオプションの範囲である。
type integerBounds struct {
	minimum, maximum int
	// kind は、断るときの文で値を呼ぶ語（"a number"、"a MiB value"）。
	kind string
}

// parse は args を前から読み、オプションと位置引数に分ける。"-" で始まる引数（"-" だけの
// ものを除く）はオプションとして読むので、一覧に無ければ断る。
func (definition commandOptions) parse(args []string) (commandArguments, error) {
	parsed := commandArguments{command: definition.command, values: make(map[string]string)}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if definition.endsAtDelimiter && argument == "--" {
			parsed.delimited = true
			parsed.rest = copyInvocationArgs(args[index+1:])
			return parsed, nil
		}
		if len(argument) < 2 || argument[0] != '-' {
			parsed.positionals = append(parsed.positionals, argument)
			continue
		}
		// = の形は長い名前だけが受ける。-j=4 は -j と 4 の書き間違いとして断る。
		name, inline, hasInline := argument, "", false
		if strings.HasPrefix(argument, "--") {
			name, inline, hasInline = strings.Cut(argument, "=")
		}
		option, known := definition.find(name)
		if !known {
			return commandArguments{}, fmt.Errorf("unknown %s option %q", definition.command, argument)
		}
		if _, duplicate := parsed.values[option.name]; duplicate {
			return commandArguments{}, fmt.Errorf("%s accepts %s only once", definition.command, option.spellings())
		}
		value := ""
		switch {
		case !option.takesValue && hasInline:
			return commandArguments{}, fmt.Errorf("%s %s does not take a value", definition.command, name)
		case hasInline:
			value = inline
		case option.takesValue:
			index++
			if index >= len(args) {
				return commandArguments{}, fmt.Errorf("%s %s requires a value", definition.command, name)
			}
			value = args[index]
		}
		parsed.values[option.name] = value
	}
	return parsed, nil
}

// parseWithoutPositionals は、オプションしか受け取らないコマンドの args を読み、位置引数があれば断る。
func (definition commandOptions) parseWithoutPositionals(args []string) (commandArguments, error) {
	arguments, err := definition.parse(args)
	if err != nil {
		return commandArguments{}, err
	}
	if len(arguments.positionals) != 0 {
		return commandArguments{}, fmt.Errorf("%s does not take %q", definition.command, arguments.positionals[0])
	}
	return arguments, nil
}

func (definition commandOptions) find(name string) (commandOption, bool) {
	for _, option := range definition.options {
		if option.name == name {
			return option, true
		}
		for _, alias := range option.aliases {
			if alias == name {
				return option, true
			}
		}
	}
	return commandOption{}, false
}

func (arguments commandArguments) has(name string) bool {
	_, found := arguments.values[name]
	return found
}

func (arguments commandArguments) hasAny(names ...string) bool {
	for _, name := range names {
		if arguments.has(name) {
			return true
		}
	}
	return false
}

// value は name の値を返す。指定されていなければ found は false になる。
func (arguments commandArguments) value(name string) (value string, found bool) {
	value, found = arguments.values[name]
	return value, found
}

// integer は name の値を bounds の範囲の整数として読む。指定されていなければ fallback を返す。
func (arguments commandArguments) integer(name string, bounds integerBounds, fallback int) (int, error) {
	value, found := arguments.values[name]
	if !found {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < bounds.minimum || parsed > bounds.maximum {
		return 0, fmt.Errorf("%s %s requires %s from %d to %d",
			arguments.command, name, bounds.kind, bounds.minimum, bounds.maximum)
	}
	return parsed, nil
}

// parseOptionalJSON は、位置引数の後ろに任意で付く --json を読む。
func parseOptionalJSON(command string, args []string) (bool, error) {
	return parseTrailingSwitch(command, args, jsonOption)
}

// parseOptionalYes は、位置引数の後ろに任意で付く -y／--yes を読む。
func parseOptionalYes(command string, args []string) (bool, error) {
	return parseTrailingSwitch(command, args, yesOption)
}

func parseTrailingSwitch(command string, args []string, option commandOption) (bool, error) {
	arguments, err := commandOptions{command: command, options: []commandOption{option}}.parseWithoutPositionals(args)
	if err != nil {
		return false, err
	}
	return arguments.has(option.name), nil
}
