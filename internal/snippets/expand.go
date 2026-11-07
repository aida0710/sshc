package snippets

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	variableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
	placeholderPattern  = regexp.MustCompile(`\{\{([A-Za-z_][A-Za-z0-9_]{0,63})\}\}`)
	snippetIDPattern    = regexp.MustCompile(`^[a-f0-9]{32}$`)
)

const secretRedaction = "[secret]"

type expansion struct {
	command string
	display string
}

type resolvedPlaceholder struct {
	start   int
	end     int
	value   string
	display string
}

// expand expands the placeholders of one command. Every variable must be used in it.
func expand(command string, variables []Variable, inputs map[string]string) (expansion, error) {
	if len(command) == 0 || len(command) > MaxCommandBytes || strings.IndexByte(command, 0) >= 0 {
		return expansion{}, ErrInvalidSnippet
	}
	expanded, err := expandTexts([]string{command}, MaxCommandBytes, variables, inputs)
	if err != nil {
		return expansion{}, err
	}
	return expanded[0], nil
}

// expandTexts expands the placeholders of several texts with the same variables, as the steps of one snippet are. Every
// variable must be used in at least one of them, and no text may grow past limit bytes, as typed or as shown.
func expandTexts(texts []string, limit int, variables []Variable, inputs map[string]string) ([]expansion, error) {
	definitions, err := variableDefinitions(variables, inputs)
	if err != nil {
		return nil, err
	}
	used := make(map[string]bool, len(definitions))
	expanded := make([]expansion, len(texts))
	for index, text := range texts {
		expanded[index], err = expandText(text, limit, definitions, inputs, used)
		if err != nil {
			return nil, err
		}
	}
	for name := range definitions {
		if !used[name] {
			return nil, fmt.Errorf("%w: unused %s", ErrInvalidVariable, name)
		}
	}
	return expanded, nil
}

// variableDefinitions checks the variables and the inputs given for them, and returns the variables by name.
func variableDefinitions(variables []Variable, inputs map[string]string) (map[string]Variable, error) {
	definitions := make(map[string]Variable, len(variables))
	for _, variable := range variables {
		if err := validateVariable(variable); err != nil {
			return nil, err
		}
		if _, exists := definitions[variable.Name]; exists {
			return nil, fmt.Errorf("%w: duplicate %s", ErrInvalidVariable, variable.Name)
		}
		definitions[variable.Name] = variable
	}
	for name, value := range inputs {
		if _, exists := definitions[name]; !exists {
			return nil, fmt.Errorf("%w: %s", ErrUnknownVariable, name)
		}
		if len(value) > MaxVariableValueBytes || strings.IndexByte(value, 0) >= 0 {
			return nil, fmt.Errorf("%w: %s", ErrInvalidVariable, name)
		}
	}
	return definitions, nil
}

// expandText puts the value of each placeholder of text in its place, and a secret's redaction in the text shown. It
// marks in used the variables it found.
func expandText(text string, limit int, definitions map[string]Variable, inputs map[string]string, used map[string]bool) (expansion, error) {
	matches := placeholderPattern.FindAllStringSubmatchIndex(text, -1)
	if malformedPlaceholders(text, matches) {
		return expansion{}, ErrMalformedTemplate
	}
	resolved := make([]resolvedPlaceholder, 0, len(matches))
	realLength, displayLength := 0, 0
	last := 0
	for _, match := range matches {
		name := text[match[2]:match[3]]
		variable, exists := definitions[name]
		if !exists {
			return expansion{}, fmt.Errorf("%w: %s", ErrUnknownVariable, name)
		}
		used[name] = true
		value, err := variableValue(variable, inputs)
		if err != nil {
			return expansion{}, err
		}
		shown := value
		if variable.Type == VariableSecret {
			shown = secretRedaction
		}
		literalLength := match[0] - last
		if !boundedExpansionLength(&realLength, limit, literalLength, len(value)) ||
			!boundedExpansionLength(&displayLength, limit, literalLength, len(shown)) {
			return expansion{}, ErrInvalidSnippet
		}
		resolved = append(resolved, resolvedPlaceholder{start: match[0], end: match[1], value: value, display: shown})
		last = match[1]
	}
	if !boundedExpansionLength(&realLength, limit, len(text)-last) ||
		!boundedExpansionLength(&displayLength, limit, len(text)-last) {
		return expansion{}, ErrInvalidSnippet
	}
	var real, display strings.Builder
	real.Grow(realLength)
	display.Grow(displayLength)
	last = 0
	for _, placeholder := range resolved {
		real.WriteString(text[last:placeholder.start])
		real.WriteString(placeholder.value)
		display.WriteString(text[last:placeholder.start])
		display.WriteString(placeholder.display)
		last = placeholder.end
	}
	real.WriteString(text[last:])
	display.WriteString(text[last:])
	return expansion{command: real.String(), display: display.String()}, nil
}

func boundedExpansionLength(total *int, limit int, lengths ...int) bool {
	for _, length := range lengths {
		if length < 0 || *total > limit-length {
			return false
		}
		*total += length
	}
	return true
}

func validateVariable(variable Variable) error {
	if !variableNamePattern.MatchString(variable.Name) || len(variable.Description) > MaxDescriptionBytes || strings.IndexByte(variable.Description, 0) >= 0 {
		return fmt.Errorf("%w: %s", ErrInvalidVariable, variable.Name)
	}
	switch variable.Type {
	case VariableString, VariableInteger, VariableBoolean, VariableSecret:
	default:
		return fmt.Errorf("%w: %s", ErrInvalidVariable, variable.Name)
	}
	if variable.Type == VariableSecret && (variable.Default != nil || !variable.Required) {
		return fmt.Errorf("%w: secret %s", ErrInvalidVariable, variable.Name)
	}
	if variable.Default != nil {
		if len(*variable.Default) > MaxVariableValueBytes || strings.IndexByte(*variable.Default, 0) >= 0 {
			return fmt.Errorf("%w: %s", ErrInvalidVariable, variable.Name)
		}
		if _, err := normaliseValue(variable.Type, *variable.Default); err != nil {
			return fmt.Errorf("%w: %s", ErrInvalidVariable, variable.Name)
		}
	}
	return nil
}

func variableValue(variable Variable, inputs map[string]string) (string, error) {
	value, present := inputs[variable.Name]
	if !present && variable.Default != nil {
		value, present = *variable.Default, true
	}
	if !present {
		if variable.Required {
			return "", fmt.Errorf("%w: %s", ErrMissingVariable, variable.Name)
		}
		return "", nil
	}
	normalised, err := normaliseValue(variable.Type, value)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrInvalidVariable, variable.Name)
	}
	return normalised, nil
}

func normaliseValue(kind VariableType, value string) (string, error) {
	switch kind {
	case VariableString, VariableSecret:
		return value, nil
	case VariableInteger:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return "", err
		}
		return strconv.FormatInt(parsed, 10), nil
	case VariableBoolean:
		if value != "true" && value != "false" {
			return "", errInvalidVariableValue
		}
		return value, nil
	default:
		return "", errInvalidVariableValue
	}
}

var errInvalidVariableValue = errors.New("invalid variable value")

func malformedPlaceholders(command string, matches [][]int) bool {
	cleaned := []byte(command)
	for _, match := range matches {
		for index := match[0]; index < match[1]; index++ {
			cleaned[index] = 'x'
		}
	}
	remaining := string(cleaned)
	return strings.Contains(remaining, "{{") || strings.Contains(remaining, "}}")
}
