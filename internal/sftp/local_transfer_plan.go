package sftp

import (
	"context"
	"io/fs"
	"os"
	"path"
)

func (s Service) PlanLocalTransfer(ctx context.Context, request RemoteTransferRequest) (_ RemoteTransferPlan, err error) {
	defer func() { err = labelLocalAccessRefusal(err) }()
	var localPath, remotePath string
	if !validTransferExclusionPatterns(request.ExcludePatterns) {
		return RemoteTransferPlan{}, ErrInvalidTransfer
	}
	if request.Operation == RemotePut {
		localPath, remotePath = request.SourcePath, request.TargetPath
	} else if request.Operation == RemoteGet {
		localPath, remotePath = request.TargetPath, request.SourcePath
	} else {
		return RemoteTransferPlan{}, ErrInvalidTransfer
	}
	if _, err := cleanLocalPath(localPath); err != nil {
		return RemoteTransferPlan{}, err
	}
	if _, err := cleanPublicPath(remotePath, false); err != nil {
		return RemoteTransferPlan{}, err
	}
	if request.Operation == RemotePut {
		root, relative, err := openLocalRoot(localPath)
		if err != nil {
			return RemoteTransferPlan{}, err
		}
		defer root.Close()
		info, err := checkLocal(root, relative, false)
		if err != nil {
			return RemoteTransferPlan{}, err
		}
		total, err := (&localTreePlanner{root: root, sourceRoot: relative, exclusions: transferExclusions(request.ExcludePatterns)}).bytes(ctx, relative, info)
		if err != nil {
			return RemoteTransferPlan{}, err
		}
		kind := TransferFile
		if info.IsDir() {
			kind = TransferFolder
		}
		return RemoteTransferPlan{Name: path.Base(relative), Kind: kind, TotalBytes: total}, nil
	}
	remote, err := s.openRequest(ctx, request.SourceAlias)
	if err != nil {
		return RemoteTransferPlan{}, err
	}
	defer remote.Close()
	info, err := remote.Lstat(remotePath)
	if err != nil {
		return RemoteTransferPlan{}, err
	}
	total := info.Size()
	kind := TransferFile
	if info.IsDir() {
		kind = TransferFolder
		total, err = treeBytes(ctx, remote, remoteTreeSelection{root: remotePath, exclusions: transferExclusions(request.ExcludePatterns)})
	} else if !info.Mode().IsRegular() {
		return RemoteTransferPlan{}, ErrUnsupportedEntry
	}
	return RemoteTransferPlan{Name: path.Base(remotePath), Kind: kind, TotalBytes: total}, err
}

type localTreePlanner struct {
	root       *os.Root
	sourceRoot string
	exclusions transferExclusions
	visited    int
}

func (planner *localTreePlanner) bytes(ctx context.Context, relative string, info fs.FileInfo) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	planner.visited++
	if planner.visited > maxTransferTreeEntries {
		return 0, ErrTraversalLimit
	}
	if info.Mode().IsRegular() {
		return info.Size(), nil
	}
	if !info.IsDir() {
		return 0, ErrUnsupportedEntry
	}
	directory, err := planner.root.Open(relative)
	if err != nil {
		return 0, err
	}
	defer directory.Close()
	infos, err := directory.Readdir(0)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, child := range infos {
		childRelative := path.Join(relative, child.Name())
		if !ValidLocalChildName(child.Name()) {
			return 0, ErrInvalidPath
		}
		if planner.exclusions.excludesChild(planner.sourceRoot, childRelative) {
			planner.visited++
			if planner.visited > maxTransferTreeEntries {
				return 0, ErrTraversalLimit
			}
			continue
		}
		checked, err := checkLocal(planner.root, childRelative, false)
		if err != nil {
			return 0, err
		}
		size, err := planner.bytes(ctx, childRelative, checked)
		if err != nil {
			return 0, err
		}
		total += size
	}
	return total, nil
}
