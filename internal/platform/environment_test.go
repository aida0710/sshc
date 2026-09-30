package platform

import (
	"runtime"
	"testing"
)

func TestLookupEnvironmentReturnsTheLastValueForAName(t *testing.T) {
	environment := []string{"PATH=/nowhere", "HOME=/home/user", "PATH=/usr/bin"}
	if value, found := LookupEnvironment(environment, "PATH"); !found || value != "/usr/bin" {
		t.Fatalf("LookupEnvironment = %q, %v; want the later PATH", value, found)
	}
	if value, found := LookupEnvironment(environment, "HOME"); !found || value != "/home/user" {
		t.Fatalf("LookupEnvironment(HOME) = %q, %v", value, found)
	}
}

func TestLookupEnvironmentDoesNotReadANameThatOnlySharesAPrefix(t *testing.T) {
	environment := []string{"PATHEXT=.EXE", "PATH_EXTRA=/opt/bin", "PATH"}
	if value, found := LookupEnvironment(environment, "PATH"); found {
		t.Fatalf("LookupEnvironment = %q; want no PATH", value)
	}
}

func TestLookupEnvironmentFindsAnEmptyValue(t *testing.T) {
	if value, found := LookupEnvironment([]string{"PATH=/usr/bin", "PATH="}, "PATH"); !found || value != "" {
		t.Fatalf("LookupEnvironment = %q, %v; want the later empty PATH", value, found)
	}
}

func TestLookupEnvironmentIgnoresNameCaseOnlyOnWindows(t *testing.T) {
	value, found := LookupEnvironment([]string{"PATH=/upper", "Path=/mixed"}, "PATH")
	want := "/upper"
	if runtime.GOOS == "windows" {
		// Windows の環境は "Path" と書くことが多く、exec もそれを PATH として渡す。
		want = "/mixed"
	}
	if !found || value != want {
		t.Fatalf("LookupEnvironment = %q, %v; want %q", value, found, want)
	}
}
