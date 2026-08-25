package remotesync

import (
	"context"
	"errors"
	"sort"
	"strings"

	"sshc/internal/envelope"
	"sshc/internal/objectstore"
)

const (
	maxHistoryGraphRevisions = 50
	maxHistoryGraphBytes     = 128 << 20
)

var ErrHistoryTarget = errors.New("the history snapshot target is not valid")

type HistoryRelation string

const (
	HistoryHead     HistoryRelation = "head"
	HistoryAncestor HistoryRelation = "ancestor"
	HistoryBranch   HistoryRelation = "branch"
	HistoryLegacy   HistoryRelation = "legacy"
)

type HistoryRevisionView struct {
	Key            string          `json:"key"`
	Revision       string          `json:"revision"`
	ParentRevision string          `json:"parentRevision,omitempty"`
	CreatedAt      string          `json:"createdAt"`
	Origin         string          `json:"origin"`
	FileCount      int             `json:"fileCount"`
	Size           int64           `json:"size"`
	LastModified   string          `json:"lastModified,omitempty"`
	Relation       HistoryRelation `json:"relation"`
	Legacy         bool            `json:"legacy"`
}

type HistoryView struct {
	CheckedAt         string                `json:"checkedAt"`
	HeadRevision      string                `json:"headRevision"`
	Revisions         []HistoryRevisionView `json:"revisions"`
	HistoryTruncated  bool                  `json:"historyTruncated"`
	DownloadTruncated bool                  `json:"downloadTruncated"`
	DownloadedBytes   int64                 `json:"downloadedBytes"`
}

type HistoryDiff struct {
	FromRevision    string   `json:"fromRevision"`
	ToRevision      string   `json:"toRevision"`
	Added           []string `json:"added"`
	Modified        []string `json:"modified"`
	Removed         []string `json:"removed"`
	DownloadedBytes int64    `json:"downloadedBytes"`
}

func openSnapshotObject(object objectstore.Object, passphrase string) (Manifest, map[string][]byte, error) {
	archive, _, err := envelope.OpenWithin(object.Body, passphrase, envelope.AcceptedFromRemote)
	if err != nil {
		return Manifest{}, nil, err
	}
	return Read(archive)
}

// History inspects a bounded recent window of encrypted history. S3 listing is
// still complete in BucketStatus; this method limits downloaded ciphertext so
// rendering a graph cannot turn a large bucket into an unbounded transfer.
func (s *Service) History(ctx context.Context, passphrase string) (HistoryView, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()

	binding, err := s.configuredBinding()
	if err != nil {
		return HistoryView{}, err
	}
	liveKey := ObjectKeyFor(binding.config)
	liveObject, err := binding.client.Get(ctx, liveKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return HistoryView{}, ErrNoSnapshot
		}
		return HistoryView{}, err
	}
	liveManifest, _, err := openSnapshotObject(liveObject, passphrase)
	if err != nil {
		return HistoryView{}, err
	}

	infos, truncated, err := binding.client.ListNewest(
		ctx, joinKey(binding.config.Path, SnapshotPrefix), maxHistoryGraphRevisions,
	)
	if err != nil {
		return HistoryView{}, err
	}
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].LastModified.Equal(infos[j].LastModified) {
			return infos[i].Key > infos[j].Key
		}
		return infos[i].LastModified.After(infos[j].LastModified)
	})

	view := HistoryView{
		CheckedAt: s.now(), HeadRevision: liveManifest.Revision,
		Revisions: []HistoryRevisionView{}, HistoryTruncated: truncated,
		DownloadedBytes: int64(len(liveObject.Body)),
	}
	manifests := map[string]Manifest{liveManifest.Revision: liveManifest}
	for _, info := range infos {
		if info.Size < 0 || view.DownloadedBytes+info.Size > maxHistoryGraphBytes {
			view.DownloadTruncated = true
			break
		}
		object, err := binding.client.Get(ctx, info.Key)
		if err != nil {
			return HistoryView{}, err
		}
		view.DownloadedBytes += int64(len(object.Body))
		manifest, _, err := openSnapshotObject(object, passphrase)
		if err != nil {
			return HistoryView{}, err
		}
		manifests[manifest.Revision] = manifest
		meta := bucketObjectView(binding.config, info)
		view.Revisions = append(view.Revisions, HistoryRevisionView{
			Key: meta.Key, Revision: manifest.Revision,
			ParentRevision: manifest.ParentRevision, CreatedAt: manifest.CreatedAt,
			Origin: manifest.Origin, FileCount: len(manifest.Files), Size: info.Size,
			LastModified: meta.LastModified, Legacy: manifest.SchemaVersion < 3,
		})
	}

	ancestors := map[string]bool{liveManifest.Revision: true}
	for revision := liveManifest.ParentRevision; revision != "" && !ancestors[revision]; {
		ancestors[revision] = true
		parent, ok := manifests[revision]
		if !ok {
			break
		}
		revision = parent.ParentRevision
	}
	for index := range view.Revisions {
		entry := &view.Revisions[index]
		switch {
		case entry.Revision == liveManifest.Revision:
			entry.Relation = HistoryHead
		case entry.Legacy:
			entry.Relation = HistoryLegacy
		case ancestors[entry.Revision]:
			entry.Relation = HistoryAncestor
		default:
			entry.Relation = HistoryBranch
		}
	}
	return view, nil
}

func historyObjectKey(config Config, key string) (string, error) {
	key = strings.TrimPrefix(key, "/")
	configuredPrefix := strings.Trim(config.Path, "/")
	if configuredPrefix != "" {
		key = strings.TrimPrefix(key, configuredPrefix+"/")
	}
	if len(key) > 1024 || !strings.HasPrefix(key, SnapshotPrefix) ||
		!strings.HasSuffix(key, "."+archiveSuffix) || strings.Contains(key, "..") ||
		strings.ContainsAny(key, "\\\r\n\x00") {
		return "", ErrHistoryTarget
	}
	return joinKey(config.Path, key), nil
}

func (s *Service) historySnapshot(ctx context.Context, binding remoteBinding, passphrase, key string) (Manifest, map[string][]byte, int64, error) {
	objectKey, err := historyObjectKey(binding.config, key)
	if err != nil {
		return Manifest{}, nil, 0, err
	}
	object, err := binding.client.Get(ctx, objectKey)
	if err != nil {
		return Manifest{}, nil, 0, err
	}
	manifest, contents, err := openSnapshotObject(object, passphrase)
	return manifest, contents, int64(len(object.Body)), err
}

// DiffHistory compares one historical snapshot with the current remote head.
// Only paths and change kinds leave the service; file contents stay in memory.
func (s *Service) DiffHistory(ctx context.Context, passphrase, key string) (HistoryDiff, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	binding, err := s.configuredBinding()
	if err != nil {
		return HistoryDiff{}, err
	}
	from, _, fromBytes, err := s.historySnapshot(ctx, binding, passphrase, key)
	if err != nil {
		return HistoryDiff{}, err
	}
	liveObject, err := binding.client.Get(ctx, ObjectKeyFor(binding.config))
	if err != nil {
		return HistoryDiff{}, err
	}
	to, _, err := openSnapshotObject(liveObject, passphrase)
	if err != nil {
		return HistoryDiff{}, err
	}
	diff := HistoryDiff{
		FromRevision: from.Revision, ToRevision: to.Revision,
		Added: []string{}, Modified: []string{}, Removed: []string{},
		DownloadedBytes: fromBytes + int64(len(liveObject.Body)),
	}
	fromEntries := entriesByPath(from.Files)
	toEntries := entriesByPath(to.Files)
	for path, target := range toEntries {
		source, exists := fromEntries[path]
		switch {
		case !exists:
			diff.Added = append(diff.Added, path)
		case source.SHA256 != target.SHA256 || source.Mode != target.Mode:
			diff.Modified = append(diff.Modified, path)
		}
	}
	for path := range fromEntries {
		if _, exists := toEntries[path]; !exists {
			diff.Removed = append(diff.Removed, path)
		}
	}
	sort.Strings(diff.Added)
	sort.Strings(diff.Modified)
	sort.Strings(diff.Removed)
	return diff, nil
}

func entriesByPath(entries []Entry) map[string]Entry {
	byPath := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		byPath[entry.Path] = entry
	}
	return byPath
}
