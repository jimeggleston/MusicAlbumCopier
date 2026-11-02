# Music Album Copier

A native Windows desktop app for selecting album folders from a music library and copying them to a USB drive for car playback. Albums are the immediate subfolders of the selected library. The app scans MP3 tags to collect genres; albums whose MP3 files have no readable genre are listed as **Unknown**.

## Run

Install Go, then run:

```powershell
go run .
```

Or build a Windows executable:

```powershell
go generate
go build -o MusicAlbumCopier.exe .
```

`go generate` embeds the Windows GUI manifest required by Walk. It requires the `go-winres` tool; install it once with `go install github.com/tc-hib/go-winres@latest`. The generated resource is included in the project, so rebuilding the executable does not otherwise require that tool.

Choose the music library, scan it, filter by genre, select albums (Ctrl+click to select multiple), and choose a destination folder on the USB drive. The entire album folder is copied, including non-MP3 files and nested subfolders. If an album folder already exists at the destination, the app asks before replacing files with matching names. A USB drive should be formatted with a file system that supports the album files' sizes (exFAT or NTFS for files larger than 4 GB).
