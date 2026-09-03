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

	token, err := tickets.Issue("session-a", 42)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("Issue() returned an empty ticket")
	}

	claim, ok := tickets.Redeem(token)
	if !ok || claim.SessionID != "session-a" || claim.Cursor != 42 {
		t.Fatalf("Redeem() = %#v, %v", claim, ok)
	}
	if _, ok := tickets.Redeem(token); ok {
		t.Fatal("the same ticket was redeemed twice")
	}
}

func TestATicketOnlyEverYieldsTheSessionItWasBoundTo(t *testing.T) {
	tickets, _ := newTickets()

	first, err := tickets.Issue("session-a", 10)
	if err != nil {
		t.Fatal(err)
	}
	second, err := tickets.Issue("session-b", 20)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two issues produced the same ticket")
	}

	if claim, _ := tickets.Redeem(first); claim.SessionID != "session-a" || claim.Cursor != 10 {
		t.Fatalf("the ticket for session-a yielded %#v", claim)
	}
	if claim, _ := tickets.Redeem(second); claim.SessionID != "session-b" || claim.Cursor != 20 {
		t.Fatalf("the ticket for session-b yielded %#v", claim)
	}
}

func TestATicketExpires(t *testing.T) {
	tickets, clock := newTickets()

	token, err := tickets.Issue("session-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	clock.advance(terminal.TicketTTL - time.Millisecond)
	fresh, err := tickets.Issue("session-b", 0)
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
	if _, err := tickets.Issue("session-a", 0); err != nil {
		t.Fatal(err)
	}
	for _, presented := range []string{"", "not-a-ticket", "0000000000000000000000000000000000000000000"} {
		if _, ok := tickets.Redeem(presented); ok {
			t.Fatalf("Redeem(%q) was accepted", presented)
		}
	}
}

func TestForgetDropsEveryTicketForOneSession(t *testing.T) {
	tickets, _ := newTickets()

	doomed, err := tickets.Issue("session-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	kept, err := tickets.Issue("session-b", 0)
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
