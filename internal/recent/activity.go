package recent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"sshc/internal/storage"
	"sshc/internal/validate"
)

// 接続の回数を UTC の1時間ごとにまとめて記録する。ターミナルの空の状態の「アクティビティ」
// （日ごとの接続回数）が読む。日の区切りは画面を見る人のタイムゾーンで決まるので、エンジンは
// 時間ごとに持ち、画面が日へまとめる。最近の接続（Store）と同じく、この端末の操作の記録で、
// 同期しない。

const (
	ActivitySchemaVersion = 1
	ActivityPathRelative  = "sshc/connection-activity.json"
	// ActivityRetention は、記録を残す期間である。画面は最大で53週（371日）を出し、タイムゾーンの
	// 差で前の日へずれる分の1日を足す。
	ActivityRetention = 372 * 24 * time.Hour
	// MaxActivityAliasesPerHour は、1時間ごとに覚える接続先の数の上限である。接続先は期間の中の
	// 台数を数えるためだけに持つので、超えた分は回数だけ数える。
	MaxActivityAliasesPerHour = 32
	activityTemporaryName     = ".connection-activity-"
)

var ErrInvalidActivity = errors.New("connection activity document is invalid")

// ActivityHour は、UTC の1時間に開いた接続の回数と、その時間に接続した接続先である。
type ActivityHour struct {
	// Hour は、その時間の始まり（UTC、分以下は0）である。
	Hour        time.Time
	Connections int
	Aliases     []string
}

type storedActivityHour struct {
	Hour        string   `json:"hour"`
	Connections int      `json:"connections"`
	Aliases     []string `json:"aliases"`
}

type activityDocument struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Hours         []storedActivityHour `json:"hours"`
}

// ActivityStore は、ひとつの engine 内の read-modify-write を直列化する。
type ActivityStore struct {
	workspace *storage.Workspace
	now       func() time.Time
	mutex     sync.Mutex
}

func NewActivityStore(workspace *storage.Workspace, now func() time.Time) *ActivityStore {
	if now == nil {
		now = time.Now
	}
	return &ActivityStore{workspace: workspace, now: now}
}

func (s *ActivityStore) Path() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(ActivityPathRelative))
}

// List は、残す期間の中の時間を、古い順に返す。
func (s *ActivityStore) List() ([]ActivityHour, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	hours, err := s.load()
	if err != nil {
		return nil, err
	}
	return s.withinRetention(hours), nil
}

// Record は、alias へ開いた接続を、今の時間に1回足す。
func (s *ActivityStore) Record(alias string) error {
	alias = strings.TrimSpace(alias)
	if alias == "" || len(alias) > validate.MaxAliasLength {
		return ErrInvalidActivity
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	hours, err := s.load()
	if err != nil {
		return err
	}
	hours = s.withinRetention(hours)
	current := s.now().UTC().Truncate(time.Hour)
	if last := len(hours) - 1; last >= 0 && hours[last].Hour.Equal(current) {
		hours[last] = countConnection(hours[last], alias)
	} else {
		hours = append(hours, countConnection(ActivityHour{Hour: current}, alias))
	}
	return s.write(hours)
}

func countConnection(hour ActivityHour, alias string) ActivityHour {
	hour.Connections++
	if !slices.Contains(hour.Aliases, alias) && len(hour.Aliases) < MaxActivityAliasesPerHour {
		hour.Aliases = append(slices.Clone(hour.Aliases), alias)
	}
	return hour
}

func (s *ActivityStore) withinRetention(hours []ActivityHour) []ActivityHour {
	oldest := s.now().UTC().Add(-ActivityRetention)
	first := 0
	for first < len(hours) && hours[first].Hour.Before(oldest) {
		first++
	}
	return hours[first:]
}

func (s *ActivityStore) load() ([]ActivityHour, error) {
	contents, err := s.workspace.FileSystem().ReadFile(s.Path())
	if errors.Is(err, fs.ErrNotExist) {
		return []ActivityHour{}, nil
	}
	if err != nil {
		return nil, err
	}
	var stored activityDocument
	if err := json.Unmarshal(contents, &stored); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidActivity, err)
	}
	if stored.SchemaVersion > ActivitySchemaVersion {
		return nil, ErrUnsupportedVersion
	}
	if stored.SchemaVersion != ActivitySchemaVersion {
		return nil, ErrInvalidActivity
	}
	hours := make([]ActivityHour, 0, len(stored.Hours))
	for _, entry := range stored.Hours {
		hour, err := parseActivityHour(entry)
		if err != nil {
			return nil, err
		}
		// 時間は古い順に1つずつ並ぶ。並びが崩れていれば、手で書き換えられたか壊れている。
		if len(hours) > 0 && !hour.Hour.After(hours[len(hours)-1].Hour) {
			return nil, ErrInvalidActivity
		}
		hours = append(hours, hour)
	}
	return hours, nil
}

func parseActivityHour(entry storedActivityHour) (ActivityHour, error) {
	hour, err := time.Parse(time.RFC3339, entry.Hour)
	if err != nil || !hour.Equal(hour.UTC().Truncate(time.Hour)) {
		return ActivityHour{}, ErrInvalidActivity
	}
	if entry.Connections < 1 || len(entry.Aliases) > MaxActivityAliasesPerHour {
		return ActivityHour{}, ErrInvalidActivity
	}
	seen := make(map[string]bool, len(entry.Aliases))
	for _, alias := range entry.Aliases {
		if strings.TrimSpace(alias) != alias || alias == "" || len(alias) > validate.MaxAliasLength || seen[alias] {
			return ActivityHour{}, ErrInvalidActivity
		}
		seen[alias] = true
	}
	return ActivityHour{Hour: hour.UTC(), Connections: entry.Connections, Aliases: entry.Aliases}, nil
}

func (s *ActivityStore) write(hours []ActivityHour) error {
	if err := s.workspace.EnsureDirectory(s.workspace.StateDir()); err != nil {
		return err
	}
	stored := make([]storedActivityHour, 0, len(hours))
	for _, hour := range hours {
		stored = append(stored, storedActivityHour{
			Hour: hour.Hour.Format(time.RFC3339), Connections: hour.Connections, Aliases: hour.Aliases,
		})
	}
	contents, err := json.MarshalIndent(activityDocument{SchemaVersion: ActivitySchemaVersion, Hours: stored}, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	return storage.WriteAtomicFile(s.workspace.FileSystem(), s.Path(), activityTemporaryName, storage.FilePermission, contents)
}
