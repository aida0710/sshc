package terminal

import (
	"reflect"
	"testing"
)

func TestOSCProgressAppliesTheBrowserStateTransitions(t *testing.T) {
	normal := &ProgramProgress{State: ProgramProgressNormal, Percent: 40}
	steps := []struct {
		name, payload string
		want          *ProgramProgress
	}{
		{"normal", "4;1;40", normal},
		{"directory report leaves progress alone", "9;/tmp", normal},
		{"missing normal percentage is ignored", "4;1", normal},
		{"negative percentage is ignored", "4;1;-1", normal},
		{"decimal percentage is ignored", "4;1;4.2", normal},
		{"four digits are ignored", "4;1;1000", normal},
		{"unknown state is ignored", "4;5;20", normal},
		{"padded state is ignored", "4;01;20", normal},
		{"error keeps percentage", "4;2", &ProgramProgress{State: ProgramProgressError, Percent: 40}},
		{"pause keeps percentage", "4;4;", &ProgramProgress{State: ProgramProgressPaused, Percent: 40}},
		{"invalid pause percentage keeps previous", "4;4;NaN", &ProgramProgress{State: ProgramProgressPaused, Percent: 40}},
		{"explicit error percentage", "4;2;75", &ProgramProgress{State: ProgramProgressError, Percent: 75}},
		{"explicit pause percentage", "4;4;90", &ProgramProgress{State: ProgramProgressPaused, Percent: 90}},
		{"clamped percentage", "4;1;999", &ProgramProgress{State: ProgramProgressNormal, Percent: 100}},
		{"zero percentage", "4;1;0;ignored", &ProgramProgress{State: ProgramProgressNormal, Percent: 0}},
		{"indeterminate", "4;3;60", &ProgramProgress{State: ProgramProgressIndeterminate}},
		{"pause after indeterminate starts at zero", "4;4", &ProgramProgress{State: ProgramProgressPaused}},
		{"clear", "4;0", nil},
		{"error after clear starts at zero", "4;2", &ProgramProgress{State: ProgramProgressError}},
	}
	for _, terminator := range []string{"\a", "\x1b\\"} {
		t.Run(terminator, func(t *testing.T) {
			var progress *ProgramProgress
			observer := newOSCObserver(nil, func(title, body string) {
				t.Fatalf("progress emitted a notification: %q %q", title, body)
			}, func(value *ProgramProgress) { progress = value })
			for _, step := range steps {
				t.Run(step.name, func(t *testing.T) {
					// Splitting at every byte covers OSC payloads and both terminators.
					for _, value := range []byte("\x1b]9;" + step.payload + terminator) {
						observer.Observe([]byte{value})
					}
					if !reflect.DeepEqual(progress, step.want) {
						t.Fatalf("progress = %+v, want %+v", progress, step.want)
					}
				})
			}
		})
	}
}

func TestCommandCompletionClearsProgressAndItsRememberedPercentage(t *testing.T) {
	for _, completion := range []string{"D", "D;0", "D;1;extra"} {
		t.Run(completion, func(t *testing.T) {
			var progress *ProgramProgress
			observer := newOSCObserver(nil, nil, func(value *ProgramProgress) { progress = value })
			observer.Observe([]byte("\x1b]9;4;1;60\a\x1b]133;A\a\x1b]133;B\a\x1b]133;C\a\x1b]133;Done\a"))
			if progress == nil || progress.Percent != 60 {
				t.Fatalf("non-completion marker cleared progress: %+v", progress)
			}
			observer.Observe([]byte("\x1b]133;" + completion + "\x1b\\"))
			if progress != nil {
				t.Fatalf("command completion left progress: %+v", progress)
			}
			observer.Observe([]byte("\x1b]9;4;4\a"))
			if progress == nil || progress.State != ProgramProgressPaused || progress.Percent != 0 {
				t.Fatalf("percentage survived completion: %+v", progress)
			}
		})
	}
}

func TestProgramProgressIgnoresOldProcessesAndClearsWhenTheShellIsReplaced(t *testing.T) {
	session := &Session{generation: 1, state: StateConnected, buffer: NewRing(32)}
	progress := &ProgramProgress{State: ProgramProgressNormal, Percent: 60}
	session.acceptProgramProgress(0, progress)
	if session.View().ProgramProgress != nil {
		t.Fatal("old process changed progress")
	}
	session.acceptProgramProgress(1, progress)
	view := session.View()
	if !reflect.DeepEqual(view.ProgramProgress, progress) {
		t.Fatalf("view = %+v", view.ProgramProgress)
	}
	view.ProgramProgress.Percent = 10
	if session.View().ProgramProgress.Percent != 60 {
		t.Fatal("mutating a view changed the session")
	}
	session.observeProcess(StateConnecting, "")
	if session.View().ProgramProgress != nil {
		t.Fatal("progress survived shell replacement")
	}
	session.acceptProgramProgress(1, progress)
	if session.View().ProgramProgress != nil {
		t.Fatal("replaced process restored progress")
	}
	session.acceptProgramProgress(session.generation, progress)
	session.acceptProgramProgress(1, nil)
	if !reflect.DeepEqual(session.View().ProgramProgress, progress) {
		t.Fatal("replaced process cleared the new process's progress")
	}
	session.acceptProgramProgress(session.generation, nil)
	session.exited = &ExitInfo{}
	session.acceptProgramProgress(session.generation, progress)
	if session.View().ProgramProgress != nil {
		t.Fatal("exited process restored progress")
	}
}
