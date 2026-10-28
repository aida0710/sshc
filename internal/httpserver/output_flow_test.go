package httpserver

import (
	"context"
	"testing"
	"time"
)

func TestOutputIsNotHeldBackForABrowserThatNeverAcknowledges(t *testing.T) {
	flow := newOutputFlow()
	flow.wrote(10 * unacknowledgedOutputLimit)

	if !flow.waitForRoom(context.Background()) {
		t.Fatal("a browser without acknowledgements was held back")
	}
}

func TestOutputWaitsUntilTheBrowserHasParsedEnough(t *testing.T) {
	flow := newOutputFlow()
	flow.wrote(unacknowledgedOutputLimit)
	flow.acknowledge(0)

	resumed := make(chan bool, 1)
	go func() { resumed <- flow.waitForRoom(context.Background()) }()
	select {
	case <-resumed:
		t.Fatal("output went on while the browser was a full window behind")
	case <-time.After(50 * time.Millisecond):
	}

	flow.acknowledge(1)
	select {
	case room := <-resumed:
		if !room {
			t.Fatal("waitForRoom gave up")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("output did not resume once the browser caught up")
	}
}

func TestAnAcknowledgementPastWhatWasSentDoesNotOpenTheWindow(t *testing.T) {
	flow := newOutputFlow()
	flow.wrote(10)
	flow.acknowledge(1 << 40)
	flow.wrote(unacknowledgedOutputLimit)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if flow.waitForRoom(ctx) {
		t.Fatal("an acknowledgement of unsent bytes let more output through")
	}
}
