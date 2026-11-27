package terminal

import (
	"runtime"
	"strings"
	"testing"

	"sshc/internal/testwait"
)

// goroutineStacksBytes は、全 goroutine のスタックを読む大きさである。この試験の goroutine は数十なので足りる。
const goroutineStacksBytes = 1 << 20

// waitingOnMutex は、function を通っている goroutine が、mutex を待って止まっているかを返す。
func waitingOnMutex(function string) bool {
	stacks := make([]byte, goroutineStacksBytes)
	length := runtime.Stack(stacks, true)
	for _, stack := range strings.Split(string(stacks[:length]), "\n\n") {
		if strings.Contains(stack, "[sync.Mutex.Lock") && strings.Contains(stack, function) {
			return true
		}
	}
	return false
}

// lookupWaits は、別の goroutine で id のセッションを引き、引けたか、mutex を待って止まったかを返す。
func lookupWaits(t *testing.T, registry *Registry, id string) bool {
	t.Helper()
	found := make(chan bool, 1)
	go func() {
		_, ok := registry.Lookup(id)
		found <- ok
	}()
	waited := false
	testwait.Until(t, func() bool {
		select {
		case ok := <-found:
			if !ok {
				t.Errorf("Lookup(%q) found nothing", id)
			}
			return true
		default:
		}
		waited = waitingOnMutex("(*Registry).Lookup")
		return waited
	})
	return waited
}

func registryOf(sessions ...*Session) *Registry {
	return &Registry{sessions: sessions}
}

// 重い出力を読むセッションは、そのあいだ自分のロックを持つ。一覧がそのセッションを待っていても、ほかの
// セッションを引く操作は待たない。
func TestABusySessionDoesNotHoldUpLookingUpAnotherWhileTheListWaitsForIt(t *testing.T) {
	busy, other := &Session{id: "busy", size: DefaultSize()}, &Session{id: "other", size: DefaultSize()}
	registry := registryOf(busy, other)
	busy.mutex.Lock()
	listed := make(chan []View, 1)
	go func() { listed <- registry.Sessions() }()
	testwait.Until(t, func() bool { return waitingOnMutex("(*Registry).Sessions") })
	waited := lookupWaits(t, registry, other.id)
	busy.mutex.Unlock()
	if views := <-listed; len(views) != 2 {
		t.Fatalf("listed %d sessions, want both", len(views))
	}
	if waited {
		t.Fatal("looking up a session waited for the list, which waited for a busy session")
	}
}

// 一覧は、各セッションの様子を聞くあいだ（View）、一覧のロックを放している。
func TestTheListAsksEachSessionWithoutHoldingUpLookups(t *testing.T) {
	asked, other := &Session{id: "asked", size: DefaultSize()}, &Session{id: "other", size: DefaultSize()}
	registry := registryOf(asked, other)
	waited := false
	// 前面のプログラムをペイン名に出すかは、View がセッションのロックを取る前に聞く。
	asked.runningCommandTitle = func() bool {
		waited = waited || lookupWaits(t, registry, other.id)
		return false
	}
	registry.Sessions()
	if waited {
		t.Fatal("looking up a session waited while the list was asking another for its view")
	}
}
