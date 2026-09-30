package httpserver

import (
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"sshc/internal/validate"
)

// hostBlockAliasOperation は、alias の上限が validate.MaxAliasLength より広い唯一の操作である。
// 起動できない alias の Host ブロックも詳細を開いて直せるよう、handler は Host ブロックの
// 上限（config_requests.go の maxAliasLength）まで受け付ける。
const hostBlockAliasOperation = "GET /api/v1/config/host"

// 経路と query の alias は、handler が validate.Alias で確かめ、長すぎれば 400 unsafe_alias で
// 断る。openapi.yaml がそれより長い alias を許すと、契約どおりの要求が断られる。
func TestAliasParametersPromiseTheLengthTheHandlersAccept(t *testing.T) {
	checked := 0
	for operation, parameters := range aliasParametersByOperation(t) {
		if operation == hostBlockAliasOperation {
			continue
		}
		for _, parameter := range parameters {
			checked++
			if parameter.Schema.MaxLength != validate.MaxAliasLength {
				t.Errorf("%s: %s の maxLength は %d だが、handler は %d 文字までしか受け付けない",
					operation, parameter.Name, parameter.Schema.MaxLength, validate.MaxAliasLength)
			}
		}
	}
	// 拾い方を壊したときに、何も比べずに通らないようにする。
	if checked < 10 {
		t.Errorf("only %d alias parameters were compared; the way they are collected is broken", checked)
	}
}

type openAPIParameter struct {
	Name   string `yaml:"name"`
	In     string `yaml:"in"`
	Schema struct {
		MaxLength int `yaml:"maxLength"`
	} `yaml:"schema"`
}

// aliasParametersByOperation は、名前に alias を含む経路と query の引数を、
// "GET /api/v1/..." の形の操作ごとに集める。経路の共通の引数も各操作に含める。
func aliasParametersByOperation(t *testing.T) map[string][]openAPIParameter {
	t.Helper()
	found := map[string][]openAPIParameter{}
	for path, item := range readOpenAPIPaths(t) {
		shared := decodeParameters(t, item["parameters"])
		for method, node := range item {
			if method == "parameters" {
				continue
			}
			var operation struct {
				Parameters yaml.Node `yaml:"parameters"`
			}
			if err := node.Decode(&operation); err != nil {
				t.Fatalf("%s %s: %v", method, path, err)
			}
			name := strings.ToUpper(method) + " " + path
			for _, parameter := range slices.Concat(shared, decodeParameters(t, operation.Parameters)) {
				if (parameter.In == "path" || parameter.In == "query") &&
					strings.Contains(strings.ToLower(parameter.Name), "alias") {
					found[name] = append(found[name], parameter)
				}
			}
		}
	}
	return found
}

func decodeParameters(t *testing.T, node yaml.Node) []openAPIParameter {
	t.Helper()
	if node.Kind == 0 {
		return nil
	}
	var parameters []openAPIParameter
	if err := node.Decode(&parameters); err != nil {
		t.Fatal(err)
	}
	return parameters
}
