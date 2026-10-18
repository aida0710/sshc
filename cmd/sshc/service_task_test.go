package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"
)

const testTaskSID = "S-1-5-21-123-456-789-1001"
const testTaskExecutable = `C:\Users\tester\Programs\sshc\sshc.exe`

type fakeTaskScheduler struct {
	t             *testing.T
	snapshot      serviceTaskSnapshot
	events        []string
	runningEngine bool
	readyError    error
	stopError     error
	afterStop     func()
}

func (scheduler *fakeTaskScheduler) Run(_ context.Context, arguments ...string) (serviceCommandResult, error) {
	scheduler.events = append(scheduler.events, arguments[0])
	switch arguments[0] {
	case "/Create":
		expectedLength := 5
		if scheduler.snapshot.exists {
			expectedLength = 6
		}
		if len(arguments) != expectedLength || scheduler.snapshot.exists && arguments[5] != "/F" {
			scheduler.t.Fatalf("unsafe overwrite flags: %q", arguments)
		}
		if arguments[1] != "/TN" || arguments[2] != "sshc" || arguments[3] != "/XML" {
			scheduler.t.Fatalf("create arguments: %q", arguments)
		}
		contents, err := os.ReadFile(arguments[4])
		if err != nil {
			scheduler.t.Fatal(err)
		}
		if len(contents)%2 != 0 || binary.LittleEndian.Uint16(contents) != 0xfeff {
			scheduler.t.Fatal("task is not UTF-16LE with a BOM")
		}
		var words []uint16
		for offset := 2; offset < len(contents); offset += 2 {
			words = append(words, binary.LittleEndian.Uint16(contents[offset:]))
		}
		task, err := parseServiceTask(string(utf16.Decode(words)))
		if err != nil {
			scheduler.t.Fatal(err)
		}
		scheduler.snapshot = serviceTaskSnapshot{exists: true, state: taskReady, definition: task}
	case "/Run":
		if !reflect.DeepEqual(arguments, []string{"/Run", "/TN", "sshc"}) {
			scheduler.t.Fatalf("run arguments: %q", arguments)
		}
		scheduler.snapshot.state = taskRunning
		scheduler.runningEngine = true
	case "/Delete":
		if !reflect.DeepEqual(arguments, []string{"/Delete", "/TN", "sshc", "/F"}) {
			scheduler.t.Fatalf("delete arguments: %q", arguments)
		}
		scheduler.snapshot = serviceTaskSnapshot{}
	default:
		scheduler.t.Fatalf("unexpected schtasks invocation: %q", arguments)
	}
	return serviceCommandResult{}, nil
}
func newTestTaskManager(t *testing.T) (*taskServiceManager, *fakeTaskScheduler) {
	t.Helper()
	scheduler := &fakeTaskScheduler{t: t}
	manager := &taskServiceManager{
		sid: testTaskSID, runner: scheduler,
		readTask:         func(context.Context) (serviceTaskSnapshot, error) { return scheduler.snapshot, nil },
		engineRunning:    func() (bool, error) { return scheduler.runningEngine, nil },
		engineResponding: func(context.Context, string) error { return scheduler.readyError },
		stopEngine: func(_ context.Context, executable string) error {
			if executable != testTaskExecutable {
				t.Fatalf("stopped another executable: %s", executable)
			}
			scheduler.events = append(scheduler.events, "stop")
			if scheduler.stopError != nil {
				return scheduler.stopError
			}
			scheduler.runningEngine = false
			scheduler.snapshot.state = taskReady
			if scheduler.afterStop != nil {
				scheduler.afterStop()
			}
			return nil
		},
		waitReady: func(_ context.Context, executable string) error {
			if executable != testTaskExecutable {
				t.Fatalf("waited for another executable: %s", executable)
			}
			scheduler.events = append(scheduler.events, "ready")
			return scheduler.readyError
		},
		lock: serviceOperationLock(t.TempDir()),
	}
	return manager, scheduler
}
func testTaskDefinition(t *testing.T) taskElement {
	t.Helper()
	contents, err := serviceTaskXML(testTaskExecutable, testTaskSID)
	if err != nil {
		t.Fatal(err)
	}
	task, err := parseServiceTask(contents)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func TestLogonTaskHasTheCurrentSIDAndRunsWithoutElevationOrATimeLimit(t *testing.T) {
	contents, err := serviceTaskXML(`C:\Users\テスト & user\sshc.exe`, testTaskSID)
	if err != nil {
		t.Fatal(err)
	}
	task, err := parseServiceTask(contents)
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		path []string
		want string
	}{
		{[]string{"RegistrationInfo", "Description"}, serviceTaskMarker},
		{[]string{"Triggers", "LogonTrigger", "UserId"}, testTaskSID},
		{[]string{"Principals", "Principal", "UserId"}, testTaskSID},
		{[]string{"Principals", "Principal", "LogonType"}, "InteractiveToken"},
		{[]string{"Principals", "Principal", "RunLevel"}, "LeastPrivilege"},
		{[]string{"Settings", "MultipleInstancesPolicy"}, "IgnoreNew"},
		{[]string{"Settings", "DisallowStartIfOnBatteries"}, "false"},
		{[]string{"Settings", "StopIfGoingOnBatteries"}, "false"},
		{[]string{"Settings", "ExecutionTimeLimit"}, "PT0S"},
		{[]string{"Settings", "Hidden"}, "false"},
		{[]string{"Settings", "RestartOnFailure", "Interval"}, serviceTaskRestartInterval},
		{[]string{"Settings", "RestartOnFailure", "Count"}, fmt.Sprint(serviceTaskRestartCount)},
		{[]string{"Actions", "Exec", "Arguments"}, "engine --background"},
	}
	for _, check := range checks {
		if got := task.at(check.path...).Text; got != check.want {
			t.Errorf("%v = %q, want %q", check.path, got, check.want)
		}
	}
	if task.executable() != `C:\Users\テスト & user\sshc.exe` || !task.managedBy(testTaskSID) {
		t.Fatalf("invalid command or ownership: %s", contents)
	}
}

func TestLogonTaskInstallRestartAndDisableUseGracefulEngineShutdown(t *testing.T) {
	manager, scheduler := newTestTaskManager(t)
	ctx := context.Background()
	if err := manager.Install(ctx, testTaskExecutable); err != nil {
		t.Fatal(err)
	}
	if state, err := manager.Status(ctx); err != nil || state != serviceActive {
		t.Fatalf("state=%v err=%v", state, err)
	}
	if enabled, err := manager.StartsAtLogin(ctx); err != nil || !enabled {
		t.Fatalf("login=%v err=%v", enabled, err)
	}
	if _, err := manager.RestartPlan(testTaskExecutable); err != nil {
		t.Fatal(err)
	}
	if restarted, err := manager.RestartIfActive(ctx, testTaskExecutable); err != nil || !restarted {
		t.Fatalf("restart=%v err=%v", restarted, err)
	}
	if removed, err := manager.Disable(ctx); err != nil || !removed {
		t.Fatalf("remove=%v err=%v", removed, err)
	}
	want := []string{"/Create", "/Run", "ready", "stop", "/Run", "ready", "stop", "/Delete"}
	if !reflect.DeepEqual(scheduler.events, want) {
		t.Fatalf("events=%q, want %q", scheduler.events, want)
	}
}

func TestLogonTaskNeverChangesAHandwrittenTaskOrAnotherUsersTask(t *testing.T) {
	for _, modification := range []string{"marker", "executable", "SID", "extra action"} {
		t.Run(modification, func(t *testing.T) {
			manager, scheduler := newTestTaskManager(t)
			contents, _ := serviceTaskXML(testTaskExecutable, testTaskSID)
			switch modification {
			case "marker":
				contents = strings.ReplaceAll(contents, serviceTaskMarker, "my own task")
			case "executable":
				contents = strings.ReplaceAll(contents, testTaskExecutable, `C:\other.exe`)
			case "SID":
				contents = strings.ReplaceAll(contents, testTaskSID, "S-1-5-18")
			case "extra action":
				contents = strings.Replace(contents, "</Actions>", "<Exec><Command>C:\\other.exe</Command></Exec></Actions>", 1)
			}
			task, err := parseServiceTask(contents)
			if err != nil {
				t.Fatal(err)
			}
			scheduler.snapshot = serviceTaskSnapshot{exists: true, state: taskRunning, definition: task}
			if err := manager.Install(context.Background(), testTaskExecutable); !errors.Is(err, errUnmanagedServiceUnit) {
				t.Fatalf("install: %v", err)
			}
			if _, err := manager.Disable(context.Background()); !errors.Is(err, errUnmanagedServiceUnit) {
				t.Fatalf("disable: %v", err)
			}
			if restarted, err := manager.RestartIfActive(context.Background(), testTaskExecutable); err != nil || restarted {
				t.Fatalf("restart=%v err=%v", restarted, err)
			}
			if len(scheduler.events) != 0 {
				t.Fatalf("changed task: %q", scheduler.events)
			}
		})
	}
}

func TestLogonTaskInstallRefusesAnAlreadyRunningEngine(t *testing.T) {
	manager, scheduler := newTestTaskManager(t)
	scheduler.runningEngine = true
	err := manager.Install(context.Background(), testTaskExecutable)
	if err == nil || !strings.Contains(err.Error(), "sshc stop") || len(scheduler.events) != 0 {
		t.Fatalf("err=%v events=%q", err, scheduler.events)
	}
}

func TestLogonTaskReportsReadyRunningAndDisabledAlongsideEngineHealth(t *testing.T) {
	for _, state := range []scheduledTaskState{taskReady, taskRunning, taskDisabled} {
		manager, scheduler := newTestTaskManager(t)
		scheduler.snapshot = serviceTaskSnapshot{exists: true, state: state, definition: testTaskDefinition(t)}
		for _, responsive := range []bool{false, true} {
			scheduler.readyError = nil
			if !responsive {
				scheduler.readyError = errors.New("not responding")
			}
			detail, err := manager.StatusDetail(context.Background())
			name := map[scheduledTaskState]string{taskReady: "Ready", taskRunning: "Running", taskDisabled: "Disabled"}[state]
			if err != nil || !strings.Contains(detail, name) || strings.Contains(detail, "not responding") == responsive {
				t.Fatalf("status=%q err=%v", detail, err)
			}
		}
		if enabled, err := manager.StartsAtLogin(context.Background()); err != nil || enabled != (state != taskDisabled) {
			t.Fatalf("enabled=%v err=%v", enabled, err)
		}
		if state != taskRunning {
			if restarted, err := manager.RestartIfActive(context.Background(), testTaskExecutable); err != nil || restarted {
				t.Fatalf("restarted=%v err=%v", restarted, err)
			}
			if len(scheduler.events) != 0 {
				t.Fatalf("started inactive task: %q", scheduler.events)
			}
		}
	}
}

func TestLogonTaskDefinitionComparisonRetainsChangedSettingsAndIgnoresSchedulerMetadata(t *testing.T) {
	for _, change := range []struct {
		old, new string
		outdated bool
	}{
		{"</RegistrationInfo>", `<URI>\sshc</URI><Date>2026-10-19T00:00:00</Date></RegistrationInfo>`, false},
		{"</Settings>", "<UseUnifiedSchedulingEngine>false</UseUnifiedSchedulingEngine></Settings>", false},
		{"</IdleSettings>", "<Duration>PT10M</Duration><WaitTimeout>PT1H</WaitTimeout></IdleSettings>", false},
		{"</Exec>", "<WorkingDirectory></WorkingDirectory></Exec>", false},
		{"</IdleSettings>", "<Duration>PT20M</Duration></IdleSettings>", true},
		{"PT0S", "PT72H", true}, {"IgnoreNew", "Parallel", true},
		{"engine --background", "engine --port 55555", true},
		{"</Triggers>", "<TimeTrigger><StartBoundary>2026-10-19T00:00:00</StartBoundary></TimeTrigger></Triggers>", true},
	} {
		manager, scheduler := newTestTaskManager(t)
		contents, _ := serviceTaskXML(testTaskExecutable, testTaskSID)
		task, err := parseServiceTask(strings.Replace(contents, change.old, change.new, 1))
		if err != nil {
			t.Fatal(err)
		}
		scheduler.snapshot = serviceTaskSnapshot{exists: true, state: taskReady, definition: task}
		if outdated, err := manager.IsDefinitionOutdated(); err != nil || outdated != change.outdated {
			t.Fatalf("%q: outdated=%v err=%v", change.new, outdated, err)
		}
	}
}

func TestLogonTaskStopsAnOperationWhenShutdownFailsOrTheDefinitionChanges(t *testing.T) {
	for _, action := range []string{"restart", "disable"} {
		for _, changed := range []bool{false, true} {
			manager, scheduler := newTestTaskManager(t)
			scheduler.snapshot = serviceTaskSnapshot{exists: true, state: taskRunning, definition: testTaskDefinition(t)}
			if changed {
				scheduler.afterStop = func() { scheduler.snapshot.definition.Children = nil }
			} else {
				scheduler.stopError = errors.New("could not verify executable")
			}
			var err error
			if action == "restart" {
				_, err = manager.RestartIfActive(context.Background(), testTaskExecutable)
			} else {
				_, err = manager.Disable(context.Background())
			}
			if err == nil || !reflect.DeepEqual(scheduler.events, []string{"stop"}) {
				t.Fatalf("%s changed=%v: err=%v events=%q", action, changed, err, scheduler.events)
			}
		}
	}
}

func TestLogonTaskPropagatesReadinessFailureAndSerializesMutations(t *testing.T) {
	manager, scheduler := newTestTaskManager(t)
	release, err := manager.lock()
	if err != nil {
		t.Fatal(err)
	}
	err = manager.Install(context.Background(), testTaskExecutable)
	_ = release()
	if err == nil || len(scheduler.events) != 0 {
		t.Fatalf("operation ignored lock: %v", err)
	}
	scheduler.readyError = errors.New("wrong executable")
	if err := manager.Install(context.Background(), testTaskExecutable); !errors.Is(err, scheduler.readyError) {
		t.Fatalf("readiness=%v", err)
	}
}

func TestLogonTaskReinstallUpdatesOnlyAManagedDefinition(t *testing.T) {
	manager, scheduler := newTestTaskManager(t)
	scheduler.snapshot = serviceTaskSnapshot{exists: true, state: taskDisabled, definition: testTaskDefinition(t)}

	if err := manager.Install(context.Background(), testTaskExecutable); err != nil {
		t.Fatal(err)
	}
	if outdated, err := manager.IsDefinitionOutdated(); err != nil || outdated {
		t.Fatalf("outdated=%v err=%v", outdated, err)
	}
	if !reflect.DeepEqual(scheduler.events, []string{"/Create", "/Run", "ready"}) {
		t.Fatalf("events=%q", scheduler.events)
	}
}

func TestLogonTaskQueryErrorsAndCancellationNeverAuthorizeAChange(t *testing.T) {
	manager, scheduler := newTestTaskManager(t)
	failure := errors.New("Task Scheduler access denied")
	manager.readTask = func(context.Context) (serviceTaskSnapshot, error) { return serviceTaskSnapshot{}, failure }
	if err := manager.Install(context.Background(), testTaskExecutable); !errors.Is(err, failure) {
		t.Fatalf("install=%v", err)
	}
	if _, err := manager.Disable(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("disable=%v", err)
	}
	if _, err := manager.RestartIfActive(context.Background(), testTaskExecutable); !errors.Is(err, failure) {
		t.Fatalf("restart=%v", err)
	}
	if len(scheduler.events) != 0 {
		t.Fatalf("changed task after a failed query: %q", scheduler.events)
	}
	ctx, cancel := context.WithCancel(context.Background())
	manager.readTask = func(context.Context) (serviceTaskSnapshot, error) {
		cancel()
		return serviceTaskSnapshot{exists: true, state: taskQueued, definition: testTaskDefinition(t)}, nil
	}
	if err := manager.waitTaskState(ctx, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("state wait=%v", err)
	}
}

func TestLogonTaskRestartDoesNotStartATaskThatStoppedAfterTheActiveCheck(t *testing.T) {
	manager, scheduler := newTestTaskManager(t)
	scheduler.snapshot = serviceTaskSnapshot{exists: true, state: taskRunning, definition: testTaskDefinition(t)}
	reads := 0
	manager.readTask = func(context.Context) (serviceTaskSnapshot, error) {
		reads++
		// The shared restart path checks the definition, active state, then definition.
		// The task exits just before the mutation's own state check.
		if reads == 4 {
			scheduler.snapshot.state = taskReady
		}
		return scheduler.snapshot, nil
	}
	if restarted, err := manager.RestartIfActive(context.Background(), testTaskExecutable); err != nil || restarted {
		t.Fatalf("restart=%v err=%v", restarted, err)
	}
	if len(scheduler.events) != 0 {
		t.Fatalf("restarted stopped task: %q", scheduler.events)
	}
}

// Windows 11 のタスクスケジューラは、登録した定義を、既定値を省き、順番を並べ替え、ログオンの
// トリガーのユーザーを SID からアカウント名に置き換えて返す。testdata は実機（Windows 11
// Enterprise、build 26100）で `schtasks /Query /TN sshc /XML` が返したものである（SID・名前・
// パスだけ置き換えた）。これを sshc の最新の定義として読めなければ、登録の直後に「変わった」と判断する。
func TestTheDefinitionReturnedByTaskSchedulerMatchesTheRegisteredOne(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("testdata", "registered-logon-task.xml"))
	if err != nil {
		t.Fatal(err)
	}
	task, err := parseServiceTask(string(contents))
	if err != nil {
		t.Fatal(err)
	}
	if task.matches(testTaskExecutable, testTaskSID) {
		t.Fatal("the account name in the logon trigger matched without being looked up")
	}
	task, err = task.withAccountSIDs(func(account string) (string, error) {
		if account != `WIN-TEST\tester` {
			return "", fmt.Errorf("unexpected account %q", account)
		}
		return testTaskSID, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !task.managedBy(testTaskSID) || !task.matches(testTaskExecutable, testTaskSID) {
		t.Fatalf("the definition returned by Task Scheduler does not match the one sshc registers:\n%s\n%s", canonicalTask(task, ""), func() string {
			expected, _ := serviceTaskXML(testTaskExecutable, testTaskSID)
			parsed, _ := parseServiceTask(expected)
			return canonicalTask(parsed, "")
		}())
	}
}
