# Music Album Copier

A cross-platform desktop app for selecting album folders from a music library and copying them to a USB drive for car playback. It uses the same graphical interface on Windows and Linux (including Debian). Albums are the immediate subfolders of the selected library. The app scans MP3 tags to collect genres; albums whose MP3 files have no readable genre are listed as **Unknown**.

## Linux (Debian)

Install Go and the Fyne build dependencies:

```sh
sudo apt install gcc pkg-config libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev
```

Open a terminal in the project folder and run:

```sh
go run .
```

Choose your music library, scan it, filter by genre, select albums, and choose a destination folder on the USB drive. Use the Cancel copy button to stop a copy in progress.

To build a Linux executable:

```sh
go build -o MusicAlbumCopier .
```

## Windows

Install Go and a GCC-compatible C compiler for CGO (for example, MinGW-w64), then run the desktop app:

```powershell
go run .
```

Or build a Windows executable on Windows:

```powershell
go build -o MusicAlbumCopier.exe .
```

## Copying albums

The entire album folder is copied, including non-MP3 files and nested subfolders. If an album folder already exists at the destination, the app asks before replacing files with matching names. A USB drive should be formatted with a file system that supports the album files' sizes (exFAT or NTFS for files larger than 4 GB).
