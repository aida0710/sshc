package remoteengine

import (
	"errors"
	"testing"
)

func TestTheEntranceKeepsThePathAndFragmentAndMovesToTheRelayPort(t *testing.T) {
	opened, err := parseEntrance([]byte("http://127.0.0.1:54447/#bootstrap=abc\n"))
	if err != nil {
		t.Fatal(err)
	}
	if opened.port != 54447 {
		t.Fatalf("remote port = %d", opened.port)
	}
	if got := opened.at(61000); got != "http://127.0.0.1:61000/#bootstrap=abc" {
		t.Fatalf("relay URL = %q", got)
	}
}

func TestAnEntranceThatDoesNotPointAtTheRemoteLoopbackIsRefused(t *testing.T) {
	for _, output := range []string{
		"",
		"http://127.0.0.1:54447/#a\nhttp://127.0.0.1:54448/#b",
		"https://127.0.0.1:54447/#bootstrap=abc",
		"http://localhost:54447/#bootstrap=abc",
		"http://10.0.0.5:54447/#bootstrap=abc",
		"http://user@127.0.0.1:54447/#bootstrap=abc",
		"http://127.0.0.1/#bootstrap=abc",
		"http://127.0.0.1:0/#bootstrap=abc",
		"http://127.0.0.1:70000/#bootstrap=abc",
		"starting the engine",
	} {
		if _, err := parseEntrance([]byte(output)); !errors.Is(err, ErrNoEntrance) {
			t.Errorf("parseEntrance(%q) = %v, want ErrNoEntrance", output, err)
		}
	}
}
