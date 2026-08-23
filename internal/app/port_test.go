package app

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
)

func fakeListen(taken map[int]bool, asked *[]int) ListenFunc {
	return func(network, address string) (net.Listener, error) {
		_, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		number, err := strconv.Atoi(port)
		if err != nil {
			return nil, err
		}
		*asked = append(*asked, number)
		if taken[number] {
			return nil, fmt.Errorf("address already in use")
		}
		return &fakeListener{}, nil
	}
}

type fakeListener struct{ net.Listener }

func (*fakeListener) Close() error   { return nil }
func (*fakeListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }

func TestAChosenPortIsTheOnlyOneTried(t *testing.T) {
	var asked []int
	if _, err := listenLoopback(fakeListen(map[int]bool{34567: true}, &asked), 34567, randomBelow); err == nil {
		t.Fatal("err = nil, want the occupied port refused")
	}
	if len(asked) != 1 || asked[0] != 34567 {
		t.Fatalf("asked = %v, want exactly the chosen port", asked)
	}
}

func TestAChosenPortIsUsedWhenItIsFree(t *testing.T) {
	var asked []int
	if _, err := listenLoopback(fakeListen(nil, &asked), 34567, randomBelow); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || asked[0] != 34567 {
		t.Fatalf("asked = %v, want exactly the chosen port", asked)
	}
}

func TestARandomPortStaysInTheRangeWeChose(t *testing.T) {
	for round := 0; round < 200; round++ {
		var asked []int
		if _, err := listenLoopback(fakeListen(nil, &asked), 0, randomBelow); err != nil {
			t.Fatal(err)
		}
		if len(asked) != 1 {
			t.Fatalf("asked = %v, want one attempt on a free machine", asked)
		}
		if asked[0] < LowestPort || asked[0] > HighestPort {
			t.Fatalf("port %d is outside %d..%d", asked[0], LowestPort, HighestPort)
		}
	}
}

func TestARandomPortIsDrawnAgainWhenItIsTaken(t *testing.T) {
	taken := map[int]bool{}
	for port := LowestPort; port <= LowestPort+4; port++ {
		taken[port] = true
	}
	draws := 0
	sequence := func(int) (int, error) {
		draws++
		if draws <= 5 {
			return draws - 1, nil
		}
		return 9, nil
	}
	var asked []int
	if _, err := listenLoopback(fakeListen(taken, &asked), 0, sequence); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 6 || asked[5] != LowestPort+9 {
		t.Fatalf("asked = %v, want it to keep drawing until one was free", asked)
	}
}

func TestDrawingGivesUpInsteadOfSpinning(t *testing.T) {
	taken := map[int]bool{}
	for port := LowestPort; port <= HighestPort; port++ {
		taken[port] = true
	}
	var asked []int
	_, err := listenLoopback(fakeListen(taken, &asked), 0, randomBelow)
	if !errors.Is(err, ErrNoFreePort) {
		t.Fatalf("err = %v, want it to give up", err)
	}
	if len(asked) != portAttempts {
		t.Fatalf("asked %d times, want %d", len(asked), portAttempts)
	}
}

func TestRefusingAChosenPortNamesIt(t *testing.T) {
	var asked []int
	_, err := listenLoopback(fakeListen(map[int]bool{34567: true}, &asked), 34567, randomBelow)
	if err == nil || !strings.Contains(err.Error(), "34567") {
		t.Fatalf("err = %v, want it to name the port", err)
	}
}
