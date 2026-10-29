package terminal

import (
	"strconv"
	"time"
)

// The browser shows when each command started and how long it took, also for
// the output it replays on attaching long after (web/src/terminal/commandBlocks.ts).
// The shell's OSC 133;C and OSC 133;D carry no time, so the engine writes the
// time it saw each of them into the output, after the chunk that held it:
//
//	OSC 7742 ; C ; <Unix milliseconds> BEL   for OSC 133;C, a command started
//	OSC 7742 ; D ; <Unix milliseconds> BEL   for OSC 133;D, a command finished
//
// The number is sshc's own. Other terminals ignore an OSC they do not know, and
// PlainTextFrom leaves it out of the text the CLI reads.
const commandTimeOSC = "7742"

// commandTime is the time the engine saw a command start ('C') or finish ('D').
type commandTime struct {
	kind byte
	at   time.Time
}

// commandTimesSequence writes the times in the order the shell reported them.
func commandTimesSequence(times []commandTime) []byte {
	var sequence []byte
	for _, mark := range times {
		sequence = append(sequence, "\x1b]"+commandTimeOSC+";"...)
		sequence = append(sequence, mark.kind, ';')
		sequence = strconv.AppendInt(sequence, mark.at.UnixMilli(), 10)
		sequence = append(sequence, '\a')
	}
	return sequence
}
