# Music Album Copier

A cross-platform desktop app for scanning MP3 albums by genre and copying selected album folders to a USB drive.

## Download and install

Download an installer for your platform from the [latest GitHub release](https://github.com/jimeggleston/MusicAlbumCopier/releases/latest):

- **Debian, Ubuntu, and MX Linux:** download [Music.Album.Copier_0.1.0_amd64.deb](https://github.com/jimeggleston/MusicAlbumCopier/releases/download/v0.1.0/Music.Album.Copier_0.1.0_amd64.deb), then install it with:

  ```sh
  sudo apt install ./Music.Album.Copier_0.1.0_amd64.deb
  ```

  Launch **Music Album Copier** from your applications menu.
- **Other Linux distributions:** download the [AppImage](https://github.com/jimeggleston/MusicAlbumCopier/releases/download/v0.1.0/Music.Album.Copier_0.1.0_amd64.AppImage), make it executable with `chmod +x Music.Album.Copier_0.1.0_amd64.AppImage`, then run `./Music.Album.Copier_0.1.0_amd64.AppImage`. Fedora-based distributions can use the [RPM package](https://github.com/jimeggleston/MusicAlbumCopier/releases/download/v0.1.0/Music.Album.Copier-0.1.0-1.x86_64.rpm).
- **Windows:** download and open the [64-bit MSI installer](https://github.com/jimeggleston/MusicAlbumCopier/releases/download/v0.1.0/Music.Album.Copier_0.1.0_x64_en-US.msi), then follow the installation prompts.

## Requirements

The requirements below are for building from source; prebuilt installers are available above.

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
