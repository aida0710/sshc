package app

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
)

// fakeListen は、掴めた番号を記録し、塞がっている番号を断る。
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

// **選ばれた番号を、選んだ人に黙って変えない。**
//
// 番号を決めた人はそれをブラウザに打つ。別の番号へ逃げれば、その綴りは外れる。
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

// **1024 未満は特権が要り、その上も名前の付いたサービスで混み合っている。**
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

// 埋まっていたら引き直す。**選んだ番号のときと違い、ここは逃げてよい** ——
// 誰も特定の番号を待っていない。
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

// **無限に引き直さない。** 3 万通りで続けて外れるなら、外れているのは前提である。
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

// 断りの綴りは、選ばれた番号を名指しする。
func TestRefusingAChosenPortNamesIt(t *testing.T) {
	var asked []int
	_, err := listenLoopback(fakeListen(map[int]bool{34567: true}, &asked), 34567, randomBelow)
	if err == nil || !strings.Contains(err.Error(), "34567") {
		t.Fatalf("err = %v, want it to name the port", err)
	}
}
