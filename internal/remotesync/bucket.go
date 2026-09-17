package remotesync

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"sshc/internal/objectstore"
)

// BucketObjectView is non-secret object metadata shown on the sync screen.
type BucketObjectView struct {
	Key          string `json:"key"`
	Size         int64  `json:"size"`
	LastModified string `json:"lastModified,omitempty"`
}

// BucketView is a live inspection of the configured bucket. History is sorted
// newest first and contains metadata only; encrypted bodies are not downloaded.
type BucketView struct {
	CheckedAt        string             `json:"checkedAt"`
	Live             *BucketObjectView  `json:"live,omitempty"`
	History          []BucketObjectView `json:"history"`
	HistoryTruncated bool               `json:"historyTruncated"`
	LocalIsLive      bool               `json:"localIsLive"`
}

const maxBucketHistoryObjects = 10000

func (s *Service) BucketStatus(ctx context.Context) (BucketView, error) {
	captured, err := s.captureHistoryRead()
	if err != nil {
		return BucketView{}, err
	}
	return s.bucketStatus(ctx, captured)
}

func (s *Service) bucketStatus(ctx context.Context, captured historyReadSnapshot) (BucketView, error) {
	binding := captured.binding
	objectKey := ObjectKeyFor(binding.config)
	view := BucketView{CheckedAt: s.now(), History: []BucketObjectView{}}
	live, err := binding.client.Stat(ctx, objectKey)
	if err != nil && !errors.Is(err, objectstore.ErrNotFound) {
		return BucketView{}, err
	}
	liveExists := err == nil
	liveETag := ""
	if err == nil {
		liveETag = live.ETag
		view.Live = bucketObjectView(binding.config, live)
		view.LocalIsLive = captured.state.target == targetID(binding.config) && captured.state.etag != "" && captured.state.etag == live.ETag
	}
	history, truncated, err := binding.client.ListNewest(ctx, joinKey(binding.config.Path, SnapshotPrefix), maxBucketHistoryObjects)
	if err != nil {
		return BucketView{}, err
	}
	view.HistoryTruncated = truncated
	sort.Slice(history, func(i, j int) bool {
		if history[i].LastModified.Equal(history[j].LastModified) {
			return history[i].Key > history[j].Key
		}
		return history[i].LastModified.After(history[j].LastModified)
	})
	for _, item := range history {
		view.History = append(view.History, *bucketObjectView(binding.config, item))
	}
	latest, latestErr := binding.client.Stat(ctx, objectKey)
	if latestErr != nil && !errors.Is(latestErr, objectstore.ErrNotFound) {
		return BucketView{}, latestErr
	}
	if (latestErr == nil) != liveExists || (latestErr == nil && latest.ETag != liveETag) {
		return BucketView{}, ErrRemoteMoved
	}
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bindingVersion != captured.bindingVersion {
		return BucketView{}, ErrRemoteMoved
	}
	current, err := s.readState()
	if err != nil {
		return BucketView{}, err
	}
	if snapshotHistoryState(current) != captured.state {
		return BucketView{}, ErrRemoteMoved
	}
	return view, nil
}

func bucketObjectView(config Config, item objectstore.ObjectInfo) *BucketObjectView {
	key := item.Key
	if prefix := strings.Trim(config.Path, "/"); prefix != "" {
		key = strings.TrimPrefix(key, prefix+"/")
	}
	modified := ""
	if !item.LastModified.IsZero() {
		modified = item.LastModified.UTC().Format(time.RFC3339)
	}
	return &BucketObjectView{Key: key, Size: item.Size, LastModified: modified}
}
