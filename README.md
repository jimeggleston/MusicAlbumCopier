# Music Album Copier

A cross-platform desktop app for scanning MP3 albums by genre and copying selected album folders to a USB drive.

## Requirements

Install Node.js 20.19 or newer, Rust 1.85 or newer, and the platform's Tauri prerequisites.

On Debian, install the desktop build dependencies:

```sh
sudo apt install build-essential curl wget file libssl-dev libxdo-dev librsvg2-dev libayatana-appindicator3-dev libwebkit2gtk-4.1-dev
```

Windows builds need the Microsoft C++ Build Tools, Rust's MSVC toolchain, Node.js, and the WebView2 runtime.

## Run and build

Install frontend dependencies:

```sh
npm ci
```

Run in development mode:

```sh
npm run tauri dev
```

Build a release bundle for the current platform:

```sh
npm run tauri build
```

Build each platform on that platform (or use a matching CI runner); Linux and Windows have different native WebView and installer dependencies.

## Features

- Scans immediate library subfolders for nested MP3s and reads their genres.
- Lists albums without readable genre tags as **Unknown**.
- Filters by genre and supports selecting multiple albums.
- Copies the complete album folder, including non-MP3 files and nested folders.
- Confirms before overwriting matching files and supports cancelling a copy.

The USB drive should use a file system that supports the album files' sizes (exFAT or NTFS for files larger than 4 GB).
