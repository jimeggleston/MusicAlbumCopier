import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import { open } from "@tauri-apps/plugin-dialog";

interface Album {
  name: string;
  path: string;
  genres: string[];
  mp3_count: number;
}

interface ScanResult {
  albums: Album[];
  warnings: string[];
}

interface CopyProgress {
  album_index: number;
  album_count: number;
  album_name: string;
}

const libraryPath = getElement<HTMLInputElement>("library-path");
const destinationPath = getElement<HTMLInputElement>("destination-path");
const scanButton = getElement<HTMLButtonElement>("scan");
const genreFilter = getElement<HTMLSelectElement>("genre-filter");
const albumCount = getElement<HTMLElement>("album-count");
const albumList = getElement<HTMLElement>("album-list");
const selectVisibleButton = getElement<HTMLButtonElement>("select-visible");
const clearSelectionButton = getElement<HTMLButtonElement>("clear-selection");
const copyButton = getElement<HTMLButtonElement>("copy");
const cancelButton = getElement<HTMLButtonElement>("cancel-copy");
const progress = getElement<HTMLProgressElement>("copy-progress");
const status = getElement<HTMLElement>("status");

let albums: Album[] = [];
const selectedPaths = new Set<string>();
let busy = false;
let copying = false;
let canCancel = false;

getElement<HTMLButtonElement>("browse-library").addEventListener("click", () => {
  void chooseFolder(libraryPath);
});
getElement<HTMLButtonElement>("browse-destination").addEventListener("click", () => {
  void chooseFolder(destinationPath);
});
scanButton.addEventListener("click", () => void scanLibrary());
genreFilter.addEventListener("change", renderAlbums);
selectVisibleButton.addEventListener("click", selectVisibleAlbums);
clearSelectionButton.addEventListener("click", clearSelection);
copyButton.addEventListener("click", () => void copySelectedAlbums());
cancelButton.addEventListener("click", () => void cancelCopy());
updateControls();

function getElement<T extends HTMLElement>(id: string): T {
  const element = document.getElementById(id);
  if (!element) {
    throw new Error(`Missing required interface element: #${id}`);
  }
  return element as T;
}

async function chooseFolder(input: HTMLInputElement): Promise<void> {
  try {
    const path = await open({ directory: true, multiple: false });
    if (typeof path === "string") {
      input.value = path;
    }
  } catch (error) {
    showError(error);
  }
}

async function scanLibrary(): Promise<void> {
  const root = libraryPath.value.trim();
  if (!root) {
    showError("Choose a music library folder.");
    return;
  }

  document.body.classList.add("scanning");
  setBusy(true);
  setStatus("Scanning ..");
  try {
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
    const result = await invoke<ScanResult>("scan_albums", { root });
    albums = result.albums;
    selectedPaths.clear();
    updateGenres();
    renderAlbums();
    const warning = result.warnings.length
      ? ` ${result.warnings.length} MP3 tag(s) could not be read; those albums may appear as Unknown.`
      : "";
    setStatus(`Found ${albums.length} album folder(s).${warning}`);
  } catch (error) {
    setStatus("Album scan failed.", true);
    showError(error);
  } finally {
    document.body.classList.remove("scanning");
    setBusy(false);
  }
}

function updateGenres(): void {
  const genres = new Set<string>();
  for (const album of albums) {
    for (const genre of album.genres) {
      genres.add(genre);
    }
  }
  const sortedGenres = [...genres].sort((a, b) => a.localeCompare(b, undefined, { sensitivity: "base" }));
  genreFilter.replaceChildren(new Option("All genres", ""));
  for (const genre of sortedGenres) {
    genreFilter.add(new Option(genre, genre));
  }
}

function visibleAlbums(): Album[] {
  const genre = genreFilter.value;
  if (!genre) {
    return albums;
  }
  return albums.filter((album) =>
    album.genres.some((albumGenre) => albumGenre.localeCompare(genre, undefined, { sensitivity: "accent" }) === 0),
  );
}

function renderAlbums(): void {
  const visible = visibleAlbums();
  albumList.replaceChildren();
  if (!visible.length) {
    const empty = document.createElement("p");
    empty.className = "empty-state";
    empty.textContent = albums.length ? "No albums match this genre." : "No album folders containing MP3 files were found.";
    albumList.append(empty);
  } else {
    for (const album of visible) {
      const label = document.createElement("label");
      label.className = "album-row";
      const checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      checkbox.checked = selectedPaths.has(album.path);
      checkbox.disabled = busy;
      checkbox.addEventListener("change", () => {
        if (checkbox.checked) {
          selectedPaths.add(album.path);
        } else {
          selectedPaths.delete(album.path);
        }
        updateSelectionSummary();
      });

      const details = document.createElement("span");
      details.className = "album-details";
      const name = document.createElement("strong");
      name.textContent = album.name;
      const metadata = document.createElement("span");
      metadata.className = "album-metadata";
      metadata.textContent = `${album.genres.join(", ")} | ${album.mp3_count} MP3${album.mp3_count === 1 ? "" : "s"}`;
      details.append(name, metadata);
      label.append(checkbox, details);
      albumList.append(label);
    }
  }
  updateSelectionSummary();
  updateControls();
}

function updateSelectionSummary(): void {
  const visibleCount = visibleAlbums().length;
  albumCount.textContent = `${visibleCount} visible · ${selectedPaths.size} selected`;
  copyButton.disabled = busy || selectedPaths.size === 0;
  selectVisibleButton.disabled = busy || visibleCount === 0;
  clearSelectionButton.disabled = busy || selectedPaths.size === 0;
}

function selectVisibleAlbums(): void {
  for (const album of visibleAlbums()) {
    selectedPaths.add(album.path);
  }
  renderAlbums();
}

function clearSelection(): void {
  selectedPaths.clear();
  renderAlbums();
}

async function copySelectedAlbums(): Promise<void> {
  const destination = destinationPath.value.trim();
  if (!destination) {
    showError("Choose a folder on your USB drive.");
    return;
  }
  const selected = albums.filter((album) => selectedPaths.has(album.path));
  if (!selected.length) {
    showError("Select at least one album to copy.");
    return;
  }

  try {
    const existing = await invoke<string[]>("existing_album_folders", {
      destination,
      albums: selected,
    });
    if (existing.length) {
      const message = `These album folders already exist and matching files will be overwritten:\n\n${existing.join("\n")}\n\nContinue?`;
      if (!window.confirm(message)) {
        return;
      }
    }
    await startCopy(destination, selected);
  } catch (error) {
    setStatus("Copy failed.", true);
    showError(error);
  }
}

async function startCopy(destination: string, selected: Album[]): Promise<void> {
  copying = true;
  setBusy(true);
  progress.value = 0;
  setStatus("Preparing to copy.");
  let unlisten: (() => void) | undefined;
  try {
    unlisten = await listen<CopyProgress>("copy-progress", ({ payload }) => {
      canCancel = true;
      updateControls();
      progress.value = ((payload.album_index - 1) / payload.album_count) * 100;
      setStatus(`Copying album ${payload.album_index} of ${payload.album_count}: ${payload.album_name}`);
    });
    await invoke("copy_albums", { destination, albums: selected });
    progress.value = 100;
    setStatus(`Copied ${selected.length} album(s) to ${destination}.`);
  } catch (error) {
    if (errorMessage(error).includes("Copy cancelled")) {
      setStatus("Copy cancelled.");
    } else {
      setStatus("Copy failed.", true);
      showError(error);
    }
  } finally {
    unlisten?.();
    copying = false;
    canCancel = false;
    setBusy(false);
  }
}

async function cancelCopy(): Promise<void> {
  if (!canCancel) {
    return;
  }
  canCancel = false;
  updateControls();
  setStatus("Cancelling copy.");
  try {
    await invoke("cancel_copy");
  } catch (error) {
    canCancel = copying;
    updateControls();
    showError(error);
  }
}

function setBusy(value: boolean): void {
  busy = value;
  updateControls();
  renderAlbums();
}

function updateControls(): void {
  libraryPath.disabled = busy;
  destinationPath.disabled = busy;
  getElement<HTMLButtonElement>("browse-library").disabled = busy;
  getElement<HTMLButtonElement>("browse-destination").disabled = busy;
  scanButton.disabled = busy;
  genreFilter.disabled = busy || albums.length === 0;
  copyButton.disabled = busy || selectedPaths.size === 0;
  cancelButton.disabled = !canCancel;
  selectVisibleButton.disabled = busy || visibleAlbums().length === 0;
  clearSelectionButton.disabled = busy || selectedPaths.size === 0;
  for (const checkbox of albumList.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')) {
    checkbox.disabled = busy;
  }
}

function setStatus(message: string, isError = false): void {
  status.textContent = message;
  status.classList.toggle("error", isError);
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function showError(error: unknown): void {
  setStatus(errorMessage(error), true);
}
