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

func expand(command string, variables []Variable, inputs map[string]string) (expansion, error) {
	if len(command) == 0 || len(command) > MaxCommandBytes || strings.IndexByte(command, 0) >= 0 {
		return expansion{}, ErrInvalidSnippet
	}
	definitions := make(map[string]Variable, len(variables))
	for _, variable := range variables {
		if err := validateVariable(variable); err != nil {
			return expansion{}, err
		}
		if _, exists := definitions[variable.Name]; exists {
			return expansion{}, fmt.Errorf("%w: duplicate %s", ErrInvalidVariable, variable.Name)
		}
		definitions[variable.Name] = variable
	}
	for name, value := range inputs {
		if _, exists := definitions[name]; !exists {
			return expansion{}, fmt.Errorf("%w: %s", ErrUnknownVariable, name)
		}
		if len(value) > MaxVariableValueBytes || strings.IndexByte(value, 0) >= 0 {
			return expansion{}, fmt.Errorf("%w: %s", ErrInvalidVariable, name)
		}
	}

	matches := placeholderPattern.FindAllStringSubmatchIndex(command, -1)
	if malformedPlaceholders(command, matches) {
		return expansion{}, ErrMalformedTemplate
	}
	used := make(map[string]bool, len(matches))
	var real, display strings.Builder
	last := 0
	for _, match := range matches {
		name := command[match[2]:match[3]]
		variable, exists := definitions[name]
		if !exists {
			return expansion{}, fmt.Errorf("%w: %s", ErrUnknownVariable, name)
		}
		used[name] = true
		value, err := variableValue(variable, inputs)
		if err != nil {
			return expansion{}, err
		}
		real.WriteString(command[last:match[0]])
		real.WriteString(value)
		display.WriteString(command[last:match[0]])
		if variable.Type == VariableSecret {
			display.WriteString(secretRedaction)
		} else {
			display.WriteString(value)
		}
		last = match[1]
	}
	real.WriteString(command[last:])
	display.WriteString(command[last:])
	for name := range definitions {
		if !used[name] {
			return expansion{}, fmt.Errorf("%w: unused %s", ErrInvalidVariable, name)
		}
	}
	return expansion{command: real.String(), display: display.String()}, nil
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
