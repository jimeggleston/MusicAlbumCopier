package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestScanAlbumsFindsMP3FoldersAndUnknownGenre(t *testing.T) {
	root := t.TempDir()
	withMP3(t, filepath.Join(root, "Artist - Album", "track.mp3"))
	if err := os.WriteFile(filepath.Join(root, "not-an-album.txt"), []byte("ignore"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "Empty folder"), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := ScanAlbums(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Albums) != 1 {
		t.Fatalf("got %d albums, want 1", len(result.Albums))
	}
	album := result.Albums[0]
	if album.Name != "Artist - Album" || album.MP3Count != 1 {
		t.Fatalf("unexpected album result: %+v", album)
	}
	if len(album.Genres) != 1 || album.Genres[0] != unknownGenre {
		t.Fatalf("got genres %v, want Unknown for an untagged MP3", album.Genres)
	}
}

func TestCopyAlbumsCopiesNestedFiles(t *testing.T) {
	sourceRoot := t.TempDir()
	albumPath := filepath.Join(sourceRoot, "Album")
	withMP3(t, filepath.Join(albumPath, "Disc 1", "track.mp3"))
	destination := t.TempDir()

	err := CopyAlbums(context.Background(), destination, []Album{{Name: "Album", Path: albumPath}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join(destination, "Album", "Disc 1", "track.mp3")
	if _, err := os.Stat(copied); err != nil {
		t.Fatalf("expected copied track at %s: %v", copied, err)
	}
}

func TestCopyAlbumsRejectsDestinationInsideSource(t *testing.T) {
	sourceRoot := t.TempDir()
	albumPath := filepath.Join(sourceRoot, "Album")
	if err := os.MkdirAll(albumPath, 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(albumPath, "USB")

	err := CopyAlbums(context.Background(), destination, []Album{{Name: "Album", Path: albumPath}}, nil)
	if err == nil {
		t.Fatal("expected overlapping source and destination to be rejected")
	}
	if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
		t.Fatalf("destination should not be created before rejecting overlap, stat error: %v", statErr)
	}
}

func withMP3(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not an ID3 file"), 0o644); err != nil {
		t.Fatal(err)
	}
}
