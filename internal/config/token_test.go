package config

import "testing"

func TestSplitArgumentsPreservesEveryByte(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		values   []string
		trailing string
	}{
		{"single", " example", []string{"example"}, ""},
		{"multiple with tabs", "\tone\ttwo  three", []string{"one", "two", "three"}, ""},
		{"quoted with spaces", ` "jump host" plain`, []string{"jump host", "plain"}, ""},
		{"empty quotes", ` ""`, []string{""}, ""},
		{"trailing whitespace", " value  \t", []string{"value"}, "  \t"},
		{"only whitespace", "   ", nil, "   "},
		{"empty", "", nil, ""},
		{"hash token is preserved", " 22 #trailing", []string{"22", "#trailing"}, ""},
		{"single quotes", ` 'bob'`, []string{"bob"}, ""},
		{"quotes inside a word", ` b"o"b`, []string{"bob"}, ""},
		{"quote closed before more text", ` "closed"tail`, []string{"closedtail"}, ""},
		{"escaped space outside quotes", ` ~/.ssh/my\ key`, []string{"~/.ssh/my key"}, ""},
		{"escaped quotes inside quotes", ` "test \"a\" = \"b\""`, []string{`test "a" = "b"`}, ""},
		{"other backslashes stay", ` C:\Users\me\.ssh\id`, []string{`C:\Users\me\.ssh\id`}, ""},
		{"single quote inside double quotes", ` "it's"`, []string{"it's"}, ""},
		{"comment keeps its own quotes", " bob # don't  ", []string{"bob", "# don't"}, "  "},
		{"hash inside a word is not a comment", " a#b", []string{"a#b"}, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			arguments, trailing, ok := splitArguments(test.input)
			if !ok {
				t.Fatalf("splitArguments(%q) reported unstructured input", test.input)
			}
			if trailing != test.trailing {
				t.Errorf("trailing = %q, want %q", trailing, test.trailing)
			}
			if len(arguments) != len(test.values) {
				t.Fatalf("arguments = %#v, want %d values", arguments, len(test.values))
			}
			rendered := ""
			for index, argument := range arguments {
				if argument.Value != test.values[index] {
					t.Errorf("value[%d] = %q, want %q", index, argument.Value, test.values[index])
				}
				rendered += argument.Lead + argument.Raw
			}
			if rendered+trailing != test.input {
				t.Fatalf("re-rendered %q, want %q", rendered+trailing, test.input)
			}
		})
	}
}

func TestSplitArgumentsRejectsQuotesThatNeverClose(t *testing.T) {
	for _, input := range []string{` "unterminated`, ` bare"quote`, ` 'single`, ` "ends with escape\"`} {
		if _, _, ok := splitArguments(input); ok {
			t.Errorf("splitArguments(%q) accepted a quote that never closes", input)
		}
	}
}

func TestRenderArgumentReadsBackAsTheSameValue(t *testing.T) {
	for _, value := range []string{
		"plain", "has space", "", "#comment", `quote"inside`, "it's", `C:\Users\me\.ssh\id`,
		`C:\dir\`, `\\server\share`, `back\"slash`, `a\ b`, "=leading", "tab\there",
	} {
		argument, err := RenderArgument(" ", value)
		if err != nil {
			t.Fatalf("RenderArgument(%q) = %v", value, err)
		}
		// 後ろに別の引数を続け、末尾の '\' が区切りの空白を食べないことも確かめる。
		arguments, _, ok := splitArguments(argument.Lead + argument.Raw + " next")
		if !ok || len(arguments) != 2 || arguments[0].Value != value || arguments[1].Value != "next" {
			t.Errorf("RenderArgument(%q) wrote %q, which reads back as %#v", value, argument.Raw, arguments)
		}
	}
}

func TestRenderArgumentRefusesLineBreaksAndNUL(t *testing.T) {
	for _, value := range []string{"line\nbreak", "carriage\rreturn", "nul\x00byte"} {
		if _, err := RenderArgument(" ", value); err == nil {
			t.Errorf("RenderArgument(%q) accepted a value a configuration line cannot hold", value)
		}
	}
}
