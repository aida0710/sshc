package sftp_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"sshc/internal/sftp"
)

func rangedService(remote *rangedFake) sftp.Service {
	return sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }}
}

func TestRemoteMediaIsReadFromWhereThePlayerSeeks(t *testing.T) {
	remote := &rangedFake{remoteWith(map[string]node{"/clip.mp4": file("clip.mp4", "0123456789", 0o644)})}
	media, err := rangedService(remote).OpenMedia(context.Background(), "edge", "/clip.mp4")
	if err != nil {
		t.Fatal(err)
	}
	defer media.Close()
	if media.ContentType != "video/mp4" {
		t.Fatalf("content type = %q", media.ContentType)
	}
	size, err := media.Content.Seek(0, io.SeekEnd)
	if err != nil || size != 10 {
		t.Fatalf("size = %d, %v", size, err)
	}
	if _, err := media.Content.Seek(6, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	tail, err := io.ReadAll(media.Content)
	if err != nil || string(tail) != "6789" {
		t.Fatalf("tail = %q, %v", tail, err)
	}
	if _, err := media.Content.Seek(2, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	head := make([]byte, 3)
	if _, err := io.ReadFull(media.Content, head); err != nil || string(head) != "234" {
		t.Fatalf("head = %q, %v", head, err)
	}
}

func TestOnlyRegularAudioAndVideoFilesOpenForPlayback(t *testing.T) {
	remote := &rangedFake{remoteWith(map[string]node{
		"/notes.txt": file("notes.txt", "text", 0o644),
		"/clips.mp4": directory("clips.mp4"),
	})}
	service := rangedService(remote)
	if _, err := service.OpenMedia(context.Background(), "edge", "/notes.txt"); !errors.Is(err, sftp.ErrPreviewType) {
		t.Fatalf("text = %v", err)
	}
	if _, err := service.OpenMedia(context.Background(), "edge", "/clips.mp4"); !errors.Is(err, sftp.ErrNotRegularFile) {
		t.Fatalf("directory = %v", err)
	}
}

func TestMediaTypesAreNamedByTheExtensionWhateverItsCase(t *testing.T) {
	for name, want := range map[string]string{"a.MP3": "audio/mpeg", "b.wav": "audio/wav", "c.MoV": "video/quicktime", "d.webm": "video/webm", "e.zip": "", "mp4": ""} {
		if got := sftp.MediaContentType(name); got != want {
			t.Fatalf("MediaContentType(%q) = %q, want %q", name, got, want)
		}
	}
}
