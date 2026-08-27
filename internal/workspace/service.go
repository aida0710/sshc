package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"time"
)

// Definition は、作成または更新時にクライアントが指定する内容である。
type Definition struct {
	Name          string
	Layout        Node
	FocusedPaneID string
}

type ReconnectState string

const ReconnectRequired ReconnectState = "reconnect_required"

// PaneReconnect は、保存したレイアウトを新しい terminal session へ結び直す要求である。
type PaneReconnect struct {
	PaneID string         `json:"paneId"`
	Alias  string         `json:"alias"`
	Kind   PaneKind       `json:"kind"`
	State  ReconnectState `json:"state"`
}

// RestorePlan は、保存されたレイアウトと、再接続が必要な pane を返す。
type RestorePlan struct {
	Workspace Workspace       `json:"workspace"`
	Panes     []PaneReconnect `json:"panes"`
}

type Service struct {
	store  *Store
	now    func() time.Time
	random io.Reader
}

func NewService(store *Store, now func() time.Time, random io.Reader) *Service {
	if now == nil {
		now = time.Now
	}
	if random == nil {
		random = rand.Reader
	}
	return &Service{store: store, now: now, random: random}
}

func (service *Service) List() ([]Workspace, error) { return service.store.List() }

func (service *Service) Get(id string) (Workspace, error) { return service.store.Get(id) }

func (service *Service) Create(definition Definition) (Workspace, error) {
	now := service.now().UTC().Format(time.RFC3339Nano)
	for range 8 {
		id, err := service.mintID()
		if err != nil {
			return Workspace{}, err
		}
		_, err = service.store.Get(id)
		if err == nil {
			continue
		}
		if !errors.Is(err, ErrNotFound) {
			return Workspace{}, err
		}
		created := Workspace{
			ID: id, Name: definition.Name, Layout: cloneNode(definition.Layout),
			FocusedPaneID: definition.FocusedPaneID, CreatedAt: now, UpdatedAt: now,
		}
		if err := service.store.Save(created); err != nil {
			return Workspace{}, err
		}
		return clone(created), nil
	}
	return Workspace{}, ErrLimit
}

func (service *Service) Update(id string, definition Definition) (Workspace, error) {
	existing, err := service.store.Get(id)
	if err != nil {
		return Workspace{}, err
	}
	existing.Name = definition.Name
	existing.Layout = cloneNode(definition.Layout)
	existing.FocusedPaneID = definition.FocusedPaneID
	existing.UpdatedAt = service.now().UTC().Format(time.RFC3339Nano)
	if err := service.store.Save(existing); err != nil {
		return Workspace{}, err
	}
	return clone(existing), nil
}

func (service *Service) Delete(id string) error { return service.store.Delete(id) }

func (service *Service) Restore(id string) (RestorePlan, error) {
	stored, err := service.store.Get(id)
	if err != nil {
		return RestorePlan{}, err
	}
	plan := RestorePlan{Workspace: stored}
	walkPanes(stored.Layout, func(pane Pane) {
		plan.Panes = append(plan.Panes, PaneReconnect{
			PaneID: pane.ID, Alias: pane.Alias, Kind: pane.EffectiveKind(), State: ReconnectRequired,
		})
	})
	return plan, nil
}

func (service *Service) mintID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := io.ReadFull(service.random, bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
