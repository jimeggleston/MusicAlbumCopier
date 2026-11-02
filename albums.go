package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dhowden/tag"
)

const unknownGenre = "Unknown"

// Album describes one immediate subfolder of the selected music library.
type Album struct {
	Name     string
	Path     string
	Genres   []string
	MP3Count int
}

// ScanResult contains discovered albums and non-fatal tag-reading warnings.
type ScanResult struct {
	Albums   []Album
	Warnings []string
}

// ScanAlbums finds immediate subfolders containing MP3 files and reads their
// genres from the files' tags.
func ScanAlbums(root string) (ScanResult, error) {
	var result ScanResult
	root = strings.TrimSpace(root)
	if root == "" {
		return result, errors.New("choose a music library folder")
	}

	info, err := os.Stat(root)
	if err != nil {
		return result, fmt.Errorf("open music library: %w", err)
	}
	if !info.IsDir() {
		return result, fmt.Errorf("music library path is not a folder: %s", root)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return result, fmt.Errorf("read music library: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		albumPath := filepath.Join(root, entry.Name())
		genres := make(map[string]string)
		album := Album{Name: entry.Name(), Path: albumPath}
		err := filepath.WalkDir(albumPath, func(path string, item fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if item.IsDir() || !strings.EqualFold(filepath.Ext(item.Name()), ".mp3") {
				return nil
			}

			album.MP3Count++
			file, err := os.Open(path)
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Could not read tags from %s: %v", path, err))
				return nil
			}
			metadata, readErr := tag.ReadFrom(file)
			closeErr := file.Close()
			if readErr != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Could not read tags from %s: %v", path, readErr))
				return nil
			}
			if closeErr != nil {
				return fmt.Errorf("close MP3 file %s: %w", path, closeErr)
			}

			genre := strings.TrimSpace(metadata.Genre())
			if genre != "" {
				key := strings.ToLower(genre)
				if _, exists := genres[key]; !exists {
					genres[key] = genre
				}
			}
			return nil
		})
		if err != nil {
			return result, fmt.Errorf("scan album %q: %w", entry.Name(), err)
		}

		if album.MP3Count == 0 {
			continue
		}
		for _, genre := range genres {
			album.Genres = append(album.Genres, genre)
		}
		if len(album.Genres) == 0 {
			album.Genres = []string{unknownGenre}
		}
		sort.Slice(album.Genres, func(i, j int) bool {
			return strings.ToLower(album.Genres[i]) < strings.ToLower(album.Genres[j])
		})
		result.Albums = append(result.Albums, album)
	}

	sort.Slice(result.Albums, func(i, j int) bool {
		return strings.ToLower(result.Albums[i].Name) < strings.ToLower(result.Albums[j].Name)
	})
	return result, nil
}

// CopyProgress reports album-level progress during a copy operation.
type CopyProgress struct {
	AlbumIndex int
	AlbumCount int
	AlbumName  string
}

// CopyAlbums copies each selected album folder under destinationRoot. Existing
// files with matching paths are replaced; the caller should confirm this with
// the user before starting the operation.
func CopyAlbums(ctx context.Context, destinationRoot string, albums []Album, progress func(CopyProgress)) error {
	if len(albums) == 0 {
		return errors.New("select at least one album to copy")
	}
	destinationRoot = strings.TrimSpace(destinationRoot)
	if destinationRoot == "" {
		return errors.New("choose a USB drive destination folder")
	}
	destinationAbs, err := filepath.Abs(destinationRoot)
	if err != nil {
		return fmt.Errorf("resolve destination folder: %w", err)
	}

	type copyPlan struct {
		name   string
		source string
		target string
	}
	plans := make([]copyPlan, 0, len(albums))
	for _, album := range albums {
		if err := ctx.Err(); err != nil {
			return err
		}
		if album.Name == "" || album.Name != filepath.Base(album.Name) || album.Name == "." || album.Name == ".." {
			return fmt.Errorf("invalid album folder name %q", album.Name)
		}
		sourceAbs, err := filepath.Abs(album.Path)
		if err != nil {
			return fmt.Errorf("resolve album %q: %w", album.Name, err)
		}
		sourceInfo, err := os.Stat(sourceAbs)
		if err != nil {
			return fmt.Errorf("open album %q: %w", album.Name, err)
		}
		if !sourceInfo.IsDir() {
			return fmt.Errorf("album path is not a folder: %s", sourceAbs)
		}

		targetAbs := filepath.Join(destinationAbs, album.Name)
		if pathsOverlap(sourceAbs, targetAbs) {
			return fmt.Errorf("destination for album %q overlaps its source folder", album.Name)
		}
		plans = append(plans, copyPlan{name: album.Name, source: sourceAbs, target: targetAbs})
	}

	if err := os.MkdirAll(destinationAbs, 0o755); err != nil {
		return fmt.Errorf("create destination folder: %w", err)
	}
	for i, plan := range plans {
		if err := ctx.Err(); err != nil {
			return err
		}
		if progress != nil {
			progress(CopyProgress{AlbumIndex: i, AlbumCount: len(plans), AlbumName: plan.name})
		}
		if err := copyDirectory(ctx, plan.source, plan.target); err != nil {
			return fmt.Errorf("copy album %q: %w", plan.name, err)
		}
	}
	return nil
}

func pathsOverlap(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if strings.EqualFold(a, b) {
		return true
	}
	within := func(parent, child string) bool {
		relative, err := filepath.Rel(parent, child)
		return err == nil && relative != "." && relative != ".." &&
			!strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
			!filepath.IsAbs(relative)
	}
	return within(a, b) || within(b, a)
}

func containsGenre(genres []string, selected string) bool {
	for _, genre := range genres {
		if strings.EqualFold(genre, selected) {
			return true
		}
	}
	return false
}

func copyDirectory(ctx context.Context, source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not supported: %s", path)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file type: %s", path)
		}
		if err := copyFile(ctx, path, target, info.Mode().Perm()); err != nil {
			return err
		}
		return os.Chtimes(target, info.ModTime(), info.ModTime())
	})
}

func copyFile(ctx context.Context, source, destination string, mode fs.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, contextReader{ctx: ctx, reader: input})
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}
