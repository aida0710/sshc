package sftp

import (
	"context"
	"strings"
)

// Existing single-entry confirmations derive evidence from the same complete
// plan used by the selection API, including the recursive tree and options.
func (s Service) ChmodActionEvidence(ctx context.Context, target string) (string, error) {
	alias, remainder, ok := strings.Cut(target, ":")
	if !ok {
		return "", ErrInvalidPath
	}
	recursive := strings.HasSuffix(remainder, ":recursive")
	if recursive {
		remainder = strings.TrimSuffix(remainder, ":recursive")
	}
	separator := strings.LastIndexByte(remainder, ':')
	if separator <= 0 || separator == len(remainder)-1 {
		return "", ErrInvalidPath
	}
	mode, err := ParseChmodMode(remainder[separator+1:])
	if err != nil {
		return "", err
	}
	entry, err := s.Stat(ctx, alias, remainder[:separator])
	if err != nil {
		return "", err
	}
	plan, err := s.PrepareChmod(ctx, ChmodRequest{Alias: alias,
		Entries: []ChmodEntry{{Path: entry.Path, ExpectedRevision: entry.Revision}},
		Options: ChmodOptions{FileMode: mode, DirectoryMode: mode, Recursive: recursive},
	})
	if err != nil {
		return "", err
	}
	defer plan.Close()
	return plan.Revision, nil
}
