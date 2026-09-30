package main

import (
	"reflect"
	"strings"
	"testing"
)

var exampleOptions = commandOptions{command: "example", options: []commandOption{
	switchOption("--json"), valueOption("--jobs", "-j"),
}}

func TestCommandOptionsReadBothValueFormsAndKeepPositionalsInOrder(t *testing.T) {
	for _, args := range [][]string{
		{"first", "--jobs", "4", "second", "--json"},
		{"first", "--jobs=4", "second", "--json"},
		{"first", "-j", "4", "second", "--json"},
	} {
		arguments, err := exampleOptions.parse(args)
		if err != nil {
			t.Fatalf("parse(%q) = %v", args, err)
		}
		if value, found := arguments.value("--jobs"); !found || value != "4" || !arguments.has("--json") {
			t.Errorf("parse(%q) values = %#v", args, arguments.values)
		}
		if !reflect.DeepEqual(arguments.positionals, []string{"first", "second"}) {
			t.Errorf("parse(%q) positionals = %q", args, arguments.positionals)
		}
	}
}

func TestCommandOptionsRefuseTheSameOptionTwiceEvenUnderAnAlias(t *testing.T) {
	for _, args := range [][]string{
		{"--json", "--json"},
		{"-j", "1", "--jobs", "2"},
		{"--jobs=1", "-j", "2"},
	} {
		_, err := exampleOptions.parse(args)
		if err == nil || !strings.Contains(err.Error(), "only once") {
			t.Errorf("parse(%q) = %v, want a refusal of the second one", args, err)
		}
	}
}

func TestCommandOptionsRefuseShapesThatCannotBeRead(t *testing.T) {
	for _, args := range [][]string{
		{"--json=true"},
		{"-j=2"},
		{"--jobs"},
		{"--unknown"},
		{"--"},
	} {
		if arguments, err := exampleOptions.parse(args); err == nil {
			t.Errorf("parse(%q) = %#v, want a refusal", args, arguments)
		}
	}
}

func TestCommandOptionsHandTheArgumentsAfterTheDelimiterThroughUnread(t *testing.T) {
	delimited := exampleOptions
	delimited.endsAtDelimiter = true
	arguments, err := delimited.parse([]string{"target", "--jobs", "--", "--", "--json", "text"})
	if err != nil {
		t.Fatal(err)
	}
	if value, _ := arguments.value("--jobs"); value != "--" {
		t.Errorf("--jobs = %q, want the delimiter spelling taken as its value", value)
	}
	if !arguments.delimited || !reflect.DeepEqual(arguments.rest, []string{"--json", "text"}) || arguments.has("--json") {
		t.Errorf("arguments = %#v", arguments)
	}
}
