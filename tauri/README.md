# Tauri application

This is the Rust/Tauri implementation of Music Album Copier. The filesystem and MP3 tag logic is isolated in `core/`; the desktop shell and commands are in `src-tauri/`.

From this directory, install frontend dependencies and launch the app in development mode:

```sh
npm install
npm run tauri dev
```

Run the portable core tests with:

```sh
cargo test --manifest-path core/Cargo.toml
```

Build a release package for the current operating system with:

```sh
npm run tauri build
```
