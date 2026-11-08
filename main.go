package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

type copierWindow struct {
	window            fyne.Window
	sourceEntry       *widget.Entry
	destinationEntry  *widget.Entry
	filterSelect      *widget.Select
	albumList         *widget.CheckGroup
	statusLabel       *widget.Label
	progressBar       *widget.ProgressBar
	scanButton        *widget.Button
	copyButton        *widget.Button
	cancelButton      *widget.Button
	sourceBrowse      *widget.Button
	destinationBrowse *widget.Button
	selectVisible     *widget.Button
	clearSelection    *widget.Button
	albums            []Album
	visible           []Album
	selected          map[string]bool
	updatingList      bool
	cancelCopy        context.CancelFunc
}

func main() {
	application := app.NewWithID("com.musicalbumcopier.app")
	window := application.NewWindow("Music Album Copier")
	window.Resize(fyne.NewSize(800, 680))

	ui := &copierWindow{
		window:   window,
		selected: make(map[string]bool),
	}
	ui.build()
	window.SetOnClosed(func() {
		if ui.cancelCopy != nil {
			ui.cancelCopy()
		}
	})
	window.ShowAndRun()
}

func (ui *copierWindow) build() {
	ui.sourceEntry = widget.NewEntry()
	ui.sourceEntry.SetPlaceHolder("Select your music library folder")
	ui.destinationEntry = widget.NewEntry()
	ui.destinationEntry.SetPlaceHolder("Select a folder on your USB drive")
	ui.filterSelect = widget.NewSelect([]string{"All genres"}, func(string) {
		ui.renderAlbums()
	})
	ui.albumList = widget.NewCheckGroup(nil, ui.selectionChanged)
	ui.statusLabel = widget.NewLabel("Choose your music library folder, then scan for albums.")
	ui.progressBar = widget.NewProgressBar()
	ui.scanButton = widget.NewButton("Scan albums", ui.scanAlbums)
	ui.copyButton = widget.NewButton("Copy selected albums", ui.copySelected)
	ui.cancelButton = widget.NewButton("Cancel copy", ui.cancelCurrentCopy)
	ui.selectVisible = widget.NewButton("Select visible", ui.selectVisibleAlbums)
	ui.clearSelection = widget.NewButton("Clear selection", ui.clearAlbums)

	ui.sourceBrowse = widget.NewButton("Browse...", func() {
		ui.browseFolder("Select your music library folder", ui.sourceEntry)
	})
	ui.destinationBrowse = widget.NewButton("Browse...", func() {
		ui.browseFolder("Select a folder on your USB drive", ui.destinationEntry)
	})
	genreRow := container.NewHBox(
		widget.NewLabel("Genre:"),
		ui.filterSelect,
		ui.selectVisible,
		ui.clearSelection,
	)
	copyRow := container.NewHBox(ui.copyButton, ui.cancelButton)
	top := container.NewVBox(
		widget.NewLabelWithStyle("Copy MP3 album folders to a USB drive", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Music library folder"),
		container.NewBorder(nil, nil, nil, ui.sourceBrowse, ui.sourceEntry),
		ui.scanButton,
		widget.NewLabel("USB drive destination folder"),
		container.NewBorder(nil, nil, nil, ui.destinationBrowse, ui.destinationEntry),
		widget.NewLabel("Filter albums by genre and select albums to copy."),
		genreRow,
	)
	bottom := container.NewVBox(copyRow, ui.progressBar, ui.statusLabel)
	ui.window.SetContent(container.NewBorder(top, bottom, nil, nil, container.NewVScroll(ui.albumList)))
	ui.setControlsEnabled(true)
}

func (ui *copierWindow) browseFolder(title string, entry *widget.Entry) {
	chooser := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil {
			ui.showError(err)
			return
		}
		if uri != nil {
			entry.SetText(uri.Path())
		}
	}, ui.window)
	chooser.SetTitleText(title)
	chooser.Resize(fyne.NewSize(720, 480))
	chooser.Show()
}

func (ui *copierWindow) scanAlbums() {
	root := strings.TrimSpace(ui.sourceEntry.Text)
	if root == "" {
		ui.showError(errors.New("choose a music library folder"))
		return
	}
	ui.setBusy(true)
	ui.statusLabel.SetText("Scanning MP3 tags...")

	go func() {
		result, err := ScanAlbums(root)
		fyne.Do(func() {
			ui.setBusy(false)
			if err != nil {
				ui.showError(err)
				ui.statusLabel.SetText("Album scan failed.")
				return
			}
			ui.albums = result.Albums
			ui.selected = make(map[string]bool)
			ui.updateGenreOptions()
			ui.renderAlbums()
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
	ui.filterSelect.SetOptions(options)
	ui.filterSelect.SetSelected("All genres")
}

func (ui *copierWindow) renderAlbums() {
	selectedGenre := ui.filterSelect.Selected
	ui.visible = make([]Album, 0)
	options := make([]string, 0)
	for _, album := range ui.albums {
		if selectedGenre != "All genres" && !containsGenre(album.Genres, selectedGenre) {
			continue
		}
		ui.visible = append(ui.visible, album)
		options = append(options, fmt.Sprintf("%s  |  %s  |  %d MP3s",
			album.Name, strings.Join(album.Genres, ", "), album.MP3Count))
	}

	selected := make([]string, 0)
	for i, album := range ui.visible {
		if ui.selected[album.Path] {
			selected = append(selected, options[i])
		}
	}
	ui.updatingList = true
	ui.albumList.Options = options
	ui.albumList.SetSelected(selected)
	ui.albumList.Refresh()
	ui.updatingList = false
}

func (ui *copierWindow) selectionChanged(options []string) {
	if ui.updatingList {
		return
	}
	for _, album := range ui.visible {
		ui.selected[album.Path] = false
	}
	selected := make(map[string]bool, len(options))
	for _, option := range options {
		selected[option] = true
	}
	for _, album := range ui.visible {
		label := fmt.Sprintf("%s  |  %s  |  %d MP3s",
			album.Name, strings.Join(album.Genres, ", "), album.MP3Count)
		ui.selected[album.Path] = selected[label]
	}
}

func (ui *copierWindow) selectVisibleAlbums() {
	for _, album := range ui.visible {
		ui.selected[album.Path] = true
	}
	ui.renderAlbums()
}

func (ui *copierWindow) clearAlbums() {
	ui.selected = make(map[string]bool)
	ui.renderAlbums()
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
	destination := strings.TrimSpace(ui.destinationEntry.Text)
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
		dialog.ShowConfirm("Overwrite existing files?", message, func(confirmed bool) {
			if confirmed {
				ui.startCopy(destination, chosen)
			}
		}, ui.window)
		return
	}
	ui.startCopy(destination, chosen)
}

func (ui *copierWindow) startCopy(destination string, chosen []Album) {
	ctx, cancel := context.WithCancel(context.Background())
	ui.cancelCopy = cancel
	ui.setBusy(true)
	ui.cancelButton.Enable()
	ui.progressBar.SetValue(0)
	ui.statusLabel.SetText("Preparing to copy...")

	go func() {
		err := CopyAlbums(ctx, destination, chosen, func(update CopyProgress) {
			fyne.Do(func() {
				ui.progressBar.SetValue(float64(update.AlbumIndex) / float64(update.AlbumCount))
				ui.statusLabel.SetText(fmt.Sprintf("Copying album %d of %d: %s",
					update.AlbumIndex+1, update.AlbumCount, update.AlbumName))
			})
		})
		fyne.Do(func() {
			cancel()
			ui.cancelCopy = nil
			ui.setBusy(false)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					ui.statusLabel.SetText("Copy cancelled.")
				} else {
					ui.showError(err)
					ui.statusLabel.SetText("Copy failed.")
				}
				return
			}
			ui.progressBar.SetValue(1)
			ui.statusLabel.SetText(fmt.Sprintf("Copied %d album(s) to %s.", len(chosen), destination))
		})
	}()
}

func (ui *copierWindow) cancelCurrentCopy() {
	if ui.cancelCopy != nil {
		ui.cancelCopy()
		ui.cancelButton.Disable()
		ui.statusLabel.SetText("Cancelling copy...")
	}
}

func (ui *copierWindow) setBusy(busy bool) {
	ui.setControlsEnabled(!busy)
}

func (ui *copierWindow) setControlsEnabled(enabled bool) {
	ui.sourceEntry.Disable()
	ui.destinationEntry.Disable()
	ui.sourceBrowse.Disable()
	ui.destinationBrowse.Disable()
	ui.filterSelect.Disable()
	ui.albumList.Disable()
	ui.scanButton.Disable()
	ui.copyButton.Disable()
	ui.selectVisible.Disable()
	ui.clearSelection.Disable()
	if enabled {
		ui.sourceEntry.Enable()
		ui.destinationEntry.Enable()
		ui.sourceBrowse.Enable()
		ui.destinationBrowse.Enable()
		ui.filterSelect.Enable()
		ui.albumList.Enable()
		ui.scanButton.Enable()
		ui.copyButton.Enable()
		ui.selectVisible.Enable()
		ui.clearSelection.Enable()
	}
	if ui.cancelCopy == nil {
		ui.cancelButton.Disable()
	}
}

func (ui *copierWindow) showError(err error) {
	dialog.ShowError(err, ui.window)
}
