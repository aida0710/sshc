package connectionlog

import (
	"context"
	"strings"
	"testing"
)

// recorded は、書かれた行を深さ付きで覚える書き先である。
type recorded struct {
	level Level
	lines []string
}

func (writer *recorded) Enabled(level Level) bool { return level <= writer.level }

func (writer *recorded) Write(level Level, message string) {
	writer.lines = append(writer.lines, string(rune('0'+level))+" "+message)
}

// 書き先が無ければ、何も書かず、何も有効でない。
func TestWithoutAWriterNothingIsWritten(t *testing.T) {
	ctx := context.Background()

	Say(ctx, Brief, "書かない")

	if Enabled(ctx, Brief) {
		t.Fatal("書き先が無いのに有効と答えた")
	}
}

// 書き先の深さより深い行は書かない。
func TestLinesDeeperThanTheWriterAreDropped(t *testing.T) {
	writer := &recorded{level: Detailed}
	ctx := With(context.Background(), writer)

	Say(ctx, Brief, "浅い %d", 1)
	Say(ctx, Detailed, "中くらい")
	Say(ctx, Full, "深い")

	if got := strings.Join(writer.lines, "|"); got != "1 浅い 1|2 中くらい" {
		t.Fatalf("lines = %q", got)
	}
	if !Enabled(ctx, Detailed) || Enabled(ctx, Full) {
		t.Fatal("Enabled が書き先の深さと合わない")
	}
}

// 書き先を重ねると、どちらにも書く。それぞれの深さで絞る。
func TestStackedWritersBothReceiveTheirLines(t *testing.T) {
	terminal := &recorded{level: Brief}
	record := &recorded{level: Full}
	ctx := With(With(context.Background(), terminal), record)

	Say(ctx, Brief, "浅い")
	Say(ctx, Full, "深い")

	if got := strings.Join(terminal.lines, "|"); got != "1 浅い" {
		t.Fatalf("terminal = %q", got)
	}
	if got := strings.Join(record.lines, "|"); got != "1 浅い|3 深い" {
		t.Fatalf("record = %q", got)
	}
	if !Enabled(ctx, Full) {
		t.Fatal("どちらかが書くなら有効と答える")
	}
}
