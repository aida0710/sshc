package recent

import "time"

// Target は、現在の SSH 設定から解決した表示用の接続先である。
type Target struct {
	Alias    string
	HostName string
	User     string
	Port     string
}

// Connection は Home が表示する最近の接続先である。
type Connection struct {
	Alias           string    `json:"alias"`
	HostName        string    `json:"hostName"`
	User            string    `json:"user"`
	Port            string    `json:"port"`
	LastConnectedAt time.Time `json:"lastConnectedAt"`
}

type Resolver func(alias string) (Target, error)

type Service struct {
	store   *Store
	resolve Resolver
}

func NewService(store *Store, resolve Resolver) *Service {
	return &Service{store: store, resolve: resolve}
}

// List は削除済みまたは現在解決できない alias を表示対象から外す。
func (s *Service) List() ([]Connection, error) {
	entries, err := s.store.List()
	if err != nil {
		return nil, err
	}
	connections := make([]Connection, 0, len(entries))
	for _, entry := range entries {
		target, err := s.resolve(entry.Alias)
		if err != nil {
			continue
		}
		connectedAt, err := time.Parse(time.RFC3339Nano, entry.LastConnectedAt)
		if err != nil {
			return nil, ErrInvalidDocument
		}
		connections = append(connections, Connection{
			Alias: target.Alias, HostName: target.HostName, User: target.User, Port: target.Port,
			LastConnectedAt: connectedAt,
		})
	}
	return connections, nil
}
