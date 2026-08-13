package terminal_test

import (
	"testing"
	"time"

	"sshc/internal/terminal"
)

type testClock struct{ at time.Time }

func (c *testClock) now() time.Time          { return c.at }
func (c *testClock) advance(d time.Duration) { c.at = c.at.Add(d) }

func newTickets() (*terminal.Tickets, *testClock) {
	clock := &testClock{at: time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)}
	return &terminal.Tickets{Now: clock.now}, clock
}

func TestATicketIsSpentByItsFirstUse(t *testing.T) {
	tickets, _ := newTickets()

	token, err := tickets.Issue("session-a")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("Issue() returned an empty ticket")
	}

	session, ok := tickets.Redeem(token)
	if !ok || session != "session-a" {
		t.Fatalf("Redeem() = %q, %v", session, ok)
	}
	if _, ok := tickets.Redeem(token); ok {
		t.Fatal("the same ticket was redeemed twice")
	}
}

// チケットはひとつのセッション ID に束縛される。別のセッションの ID が
// これで開けるようなら、一覧を読めた者は誰の端末へでも繋げることになる。
func TestATicketOnlyEverYieldsTheSessionItWasBoundTo(t *testing.T) {
	tickets, _ := newTickets()

	first, err := tickets.Issue("session-a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := tickets.Issue("session-b")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two issues produced the same ticket")
	}

	if session, _ := tickets.Redeem(first); session != "session-a" {
		t.Fatalf("the ticket for session-a yielded %q", session)
	}
	if session, _ := tickets.Redeem(second); session != "session-b" {
		t.Fatalf("the ticket for session-b yielded %q", session)
	}
}

func TestATicketExpires(t *testing.T) {
	tickets, clock := newTickets()

	token, err := tickets.Issue("session-a")
	if err != nil {
		t.Fatal(err)
	}
	// 期限の内側では通る。境界そのものは通らない側に倒す。
	clock.advance(terminal.TicketTTL - time.Millisecond)
	fresh, err := tickets.Issue("session-b")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tickets.Redeem(token); !ok {
		t.Fatal("a ticket inside its lifetime was refused")
	}

	clock.advance(terminal.TicketTTL)
	if _, ok := tickets.Redeem(fresh); ok {
		t.Fatal("a ticket past its lifetime was accepted")
	}
}

func TestAnUnknownOrEmptyTicketIsRefused(t *testing.T) {
	tickets, _ := newTickets()
	if _, err := tickets.Issue("session-a"); err != nil {
		t.Fatal(err)
	}
	for _, presented := range []string{"", "not-a-ticket", "0000000000000000000000000000000000000000000"} {
		if _, ok := tickets.Redeem(presented); ok {
			t.Fatalf("Redeem(%q) was accepted", presented)
		}
	}
}

// 閉じられたセッションのために発行されたチケットは残らない。使われなかった
// 認可が、もう存在しないものを指したまま宙に浮くことがない。
func TestForgetDropsEveryTicketForOneSession(t *testing.T) {
	tickets, _ := newTickets()

	doomed, err := tickets.Issue("session-a")
	if err != nil {
		t.Fatal(err)
	}
	kept, err := tickets.Issue("session-b")
	if err != nil {
		t.Fatal(err)
	}

	tickets.Forget("session-a")
	if _, ok := tickets.Redeem(doomed); ok {
		t.Fatal("a forgotten ticket was still accepted")
	}
	if _, ok := tickets.Redeem(kept); !ok {
		t.Fatal("Forget() dropped another session's ticket")
	}
}
