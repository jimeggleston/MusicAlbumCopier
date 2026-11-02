package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

type copierWindow struct {
	mainWindow        *walk.MainWindow
	sourceEntry       *walk.LineEdit
	destinationEntry  *walk.LineEdit
	filterCombo       *walk.ComboBox
	albumList         *walk.ListBox
	statusLabel       *walk.Label
	progressBar       *walk.ProgressBar
	scanButton        *walk.PushButton
	copyButton        *walk.PushButton
	cancelButton      *walk.PushButton
	sourceBrowse      *walk.PushButton
	destinationBrowse *walk.PushButton
	selectVisible     *walk.PushButton
	clearSelection    *walk.PushButton

	albums       []Album
	visible      []Album
	selected     map[string]bool
	updatingList bool
	cancelCopy   context.CancelFunc
}

func main() {
	ui := &copierWindow{selected: make(map[string]bool)}
	if _, err := (MainWindow{
		AssignTo: &ui.mainWindow,
		Title:    "Music Album Copier",
		MinSize:  Size{Width: 720, Height: 580},
		Size:     Size{Width: 780, Height: 680},
		Layout:   VBox{},
		Children: []Widget{
			Label{
				Text:      "Copy MP3 album folders to a USB drive",
				Font:      Font{Family: "Segoe UI", PointSize: 12, Bold: true},
				Alignment: AlignHNearVCenter,
			},
			Label{Text: "Music library folder"},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					LineEdit{AssignTo: &ui.sourceEntry},
					PushButton{AssignTo: &ui.sourceBrowse, Text: "Browse...", OnClicked: ui.browseSource},
				},
			},
			PushButton{AssignTo: &ui.scanButton, Text: "Scan albums", OnClicked: ui.scanAlbums},
			Label{Text: "USB drive destination folder"},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					LineEdit{AssignTo: &ui.destinationEntry},
					PushButton{AssignTo: &ui.destinationBrowse, Text: "Browse...", OnClicked: ui.browseDestination},
				},
			},
			Label{Text: "Filter albums by genre. Use Ctrl+click to select multiple albums."},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					Label{Text: "Genre:"},
					ComboBox{
						AssignTo: &ui.filterCombo,
						Model:    []string{"All genres"},
						OnCurrentIndexChanged: func() {
							ui.renderAlbums()
						},
					},
					PushButton{AssignTo: &ui.selectVisible, Text: "Select visible", OnClicked: ui.selectVisibleAlbums},
					PushButton{AssignTo: &ui.clearSelection, Text: "Clear selection", OnClicked: ui.clearAlbums},
				},
			},
			ListBox{
				AssignTo:                 &ui.albumList,
				Model:                    []string{},
				MultiSelection:           true,
				StretchFactor:            1,
				OnSelectedIndexesChanged: ui.selectionChanged,
			},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					PushButton{AssignTo: &ui.copyButton, Text: "Copy selected albums", OnClicked: ui.copySelected},
					PushButton{AssignTo: &ui.cancelButton, Text: "Cancel copy", OnClicked: ui.cancelCurrentCopy},
				},
			},
			ProgressBar{AssignTo: &ui.progressBar, MinValue: 0, MaxValue: 100},
			Label{AssignTo: &ui.statusLabel, Text: "Choose your music library folder, then scan for albums."},
		},
	}).Run(); err != nil {
		log.Fatal(err)
	}
	if ui.cancelCopy != nil {
		ui.cancelCopy()
	}
}

func (ui *copierWindow) browseSource() {
	dialog := &walk.FileDialog{Title: "Select your music library folder", InitialDirPath: ui.sourceEntry.Text()}
	accepted, err := dialog.ShowBrowseFolder(ui.mainWindow)
	if err != nil {
		ui.showError(err)
		return
	}
	if accepted {
		ui.sourceEntry.SetText(dialog.FilePath)
	}
}

func (ui *copierWindow) browseDestination() {
	dialog := &walk.FileDialog{Title: "Select a folder on your USB drive", InitialDirPath: ui.destinationEntry.Text()}
	accepted, err := dialog.ShowBrowseFolder(ui.mainWindow)
	if err != nil {
		ui.showError(err)
		return
	}
	if accepted {
		ui.destinationEntry.SetText(dialog.FilePath)
	}
}

func (ui *copierWindow) scanAlbums() {
	root := strings.TrimSpace(ui.sourceEntry.Text())
	if root == "" {
		ui.showError(errors.New("choose a music library folder"))
		return
	}
	ui.setControlsEnabled(false)
	ui.statusLabel.SetText("Scanning MP3 tags...")

	go func() {
		result, err := ScanAlbums(root)
		ui.mainWindow.Synchronize(func() {
			ui.setControlsEnabled(true)
			if err != nil {
				ui.showError(err)
				ui.statusLabel.SetText("Album scan failed.")
				return
			}
			ui.albums = result.Albums
			ui.selected = make(map[string]bool)
			ui.updateGenreOptions()
			if err := ui.renderAlbums(); err != nil {
				ui.showError(err)
				ui.statusLabel.SetText("Could not display scanned albums.")
				return
			}
			message := fmt.Sprintf("Found %d album folders.", len(ui.albums))
			if len(result.Warnings) > 0 {
				message += fmt.Sprintf(" %d MP3 tag(s) could not be read; those albums may appear as Unknown.", len(result.Warnings))
			}
			ui.statusLabel.SetText(message)
		})
	}()
}

func (ui *copierWindow) updateGenreOptions() {
	genres := make(map[string]bool)
	for _, album := range ui.albums {
		for _, genre := range album.Genres {
			genres[genre] = true
		}
	}
	options := []string{"All genres"}
	for genre := range genres {
		options = append(options, genre)
	}
	sort.Slice(options[1:], func(i, j int) bool {
		return strings.ToLower(options[i+1]) < strings.ToLower(options[j+1])
	})
	ui.filterCombo.SetModel(options)
	ui.filterCombo.SetCurrentIndex(0)
}

func (ui *copierWindow) renderAlbums() error {
	selectedGenre := ui.filterCombo.Text()
	ui.visible = make([]Album, 0)
	items := make([]string, 0)
	for _, album := range ui.albums {
		if selectedGenre != "All genres" && !containsGenre(album.Genres, selectedGenre) {
			continue
		}
		ui.visible = append(ui.visible, album)
		items = append(items, fmt.Sprintf("%s  |  %s  |  %d MP3s", album.Name, strings.Join(album.Genres, ", "), album.MP3Count))
	}

	ui.updatingList = true
	defer func() { ui.updatingList = false }()
	if err := ui.albumList.SetModel(items); err != nil {
		return err
	}
	indexes := make([]int, 0)
	for i, album := range ui.visible {
		if ui.selected[album.Path] {
			indexes = append(indexes, i)
		}
	}
	ui.albumList.SetSelectedIndexes(indexes)
	return nil
}

func (ui *copierWindow) selectionChanged() {
	if ui.updatingList {
		return
	}
	for _, album := range ui.visible {
		ui.selected[album.Path] = false
	}
	for _, index := range ui.albumList.SelectedIndexes() {
		if index >= 0 && index < len(ui.visible) {
			ui.selected[ui.visible[index].Path] = true
		}
	}
}

func (ui *copierWindow) selectVisibleAlbums() {
	for _, album := range ui.visible {
		ui.selected[album.Path] = true
	}
	if err := ui.renderAlbums(); err != nil {
		ui.showError(err)
	}
}

func (ui *copierWindow) clearAlbums() {
	ui.selected = make(map[string]bool)
	if err := ui.renderAlbums(); err != nil {
		ui.showError(err)
	}
}

func (ui *copierWindow) copySelected() {
	chosen := make([]Album, 0)
	for _, album := range ui.albums {
		if ui.selected[album.Path] {
			chosen = append(chosen, album)
		}
	}
	if len(chosen) == 0 {
		ui.showError(errors.New("select at least one album to copy"))
		return
	}
	destination := strings.TrimSpace(ui.destinationEntry.Text())
	if destination == "" {
		ui.showError(errors.New("choose a folder on your USB drive"))
		return
	}
	existing, err := existingAlbumFolders(destination, chosen)
	if err != nil {
		ui.showError(err)
		return
	}
	if len(existing) > 0 {
		message := "These album folders already exist and matching files will be overwritten:\n\n" +
			strings.Join(existing, "\n") + "\n\nContinue?"
		if walk.MsgBox(ui.mainWindow, "Overwrite existing files?", message,
			walk.MsgBoxYesNo|walk.MsgBoxIconWarning) != walk.DlgCmdYes {
			return
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	ui.cancelCopy = cancel
	ui.setControlsEnabled(false)
	ui.cancelButton.SetEnabled(true)
	ui.progressBar.SetValue(0)
	ui.statusLabel.SetText("Preparing to copy...")

	go func() {
		err := CopyAlbums(ctx, destination, chosen, func(update CopyProgress) {
			ui.mainWindow.Synchronize(func() {
				ui.progressBar.SetValue(update.AlbumIndex * 100 / update.AlbumCount)
				ui.statusLabel.SetText(fmt.Sprintf("Copying album %d of %d: %s", update.AlbumIndex+1, update.AlbumCount, update.AlbumName))
			})
		})
		ui.mainWindow.Synchronize(func() {
			cancel()
			ui.cancelCopy = nil
			ui.setControlsEnabled(true)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					ui.statusLabel.SetText("Copy cancelled.")
				} else {
					ui.showError(err)
					ui.statusLabel.SetText("Copy failed.")
				}
				return
			}
			ui.progressBar.SetValue(100)
			ui.statusLabel.SetText(fmt.Sprintf("Copied %d album(s) to %s.", len(chosen), destination))
		})
	}()
}

func (ui *copierWindow) cancelCurrentCopy() {
	if ui.cancelCopy != nil {
		ui.cancelCopy()
		ui.cancelButton.SetEnabled(false)
		ui.statusLabel.SetText("Cancelling copy...")
	}
}

func (ui *copierWindow) setControlsEnabled(enabled bool) {
	ui.sourceEntry.SetEnabled(enabled)
	ui.sourceBrowse.SetEnabled(enabled)
	ui.scanButton.SetEnabled(enabled)
	ui.destinationEntry.SetEnabled(enabled)
	ui.destinationBrowse.SetEnabled(enabled)
	ui.filterCombo.SetEnabled(enabled)
	ui.albumList.SetEnabled(enabled)
	ui.selectVisible.SetEnabled(enabled)
	ui.clearSelection.SetEnabled(enabled)
	ui.copyButton.SetEnabled(enabled)
	ui.cancelButton.SetEnabled(!enabled && ui.cancelCopy != nil)
}

func (ui *copierWindow) showError(err error) {
	walk.MsgBox(ui.mainWindow, "Music Album Copier", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
}

func existingAlbumFolders(destination string, albums []Album) ([]string, error) {
	var existing []string
	for _, album := range albums {
		path := filepath.Join(destination, album.Name)
		_, err := os.Stat(path)
		if err == nil {
			existing = append(existing, album.Name)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("check destination %s: %w", path, err)
		}
	}
	return existing, nil
}
