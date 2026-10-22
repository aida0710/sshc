package sftp

import (
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"testing"

	pkgsftp "github.com/pkg/sftp"
)

// endlessLister は、一覧を終わらせない SFTP サーバーとして、毎回新しい名前を返し続ける。
type endlessLister struct{ listed *int }

func (lister endlessLister) ListAt(destination []os.FileInfo, _ int64) (int, error) {
	for index := range destination {
		*lister.listed++
		destination[index] = fixtureInfo{name: "entry-" + strconv.Itoa(*lister.listed)}
	}
	return len(destination), nil
}

func (lister endlessLister) Filelist(*pkgsftp.Request) (pkgsftp.ListerAt, error) {
	return lister, nil
}

// fixedLister は、決まった数の名前を返して一覧を終える。
type fixedLister int

func (count fixedLister) ListAt(destination []os.FileInfo, offset int64) (int, error) {
	written := 0
	for ; written < len(destination) && offset+int64(written) < int64(count); written++ {
		destination[written] = fixtureInfo{name: "entry-" + strconv.FormatInt(offset+int64(written), 10)}
	}
	if offset+int64(written) >= int64(count) {
		return written, io.EOF
	}
	return written, nil
}

func (count fixedLister) Filelist(*pkgsftp.Request) (pkgsftp.ListerAt, error) {
	return count, nil
}

func directoryLimitClient(t *testing.T, lister pkgsftp.FileLister) *Client {
	t.Helper()
	serverConnection, clientConnection := net.Pipe()
	server := pkgsftp.NewRequestServer(serverConnection, pkgsftp.Handlers{FileList: lister})
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Serve(); _ = server.Close() }()
	client, err := newClientPipe(clientConnection, clientConnection)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close(); <-done })
	return client
}

func TestReadDirStopsAtTheEntryLimitWhenTheServerNeverEndsTheListing(t *testing.T) {
	listed := 0
	client := directoryLimitClient(t, endlessLister{listed: &listed})
	infos, err := client.ReadDir(t.Context(), "/endless")
	if !errors.Is(err, ErrTraversalLimit) {
		t.Fatalf("ReadDir() = %d entries, %v; want %v", len(infos), err, ErrTraversalLimit)
	}
	if listed > 2*maxDirectoryEntries {
		t.Fatalf("server listed %d entries before the limit stopped the listing", listed)
	}
}

func TestReadDirListsAFolderUpToTheEntryLimitAndCanListItAgain(t *testing.T) {
	client := directoryLimitClient(t, fixedLister(maxDirectoryEntries))
	for range 2 {
		infos, err := client.ReadDir(t.Context(), "/large")
		if err != nil {
			t.Fatal(err)
		}
		if len(infos) != maxDirectoryEntries {
			t.Fatalf("ReadDir() = %d entries, want %d", len(infos), maxDirectoryEntries)
		}
	}
}
