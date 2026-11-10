use lofty::{file::TaggedFileExt, probe::read_from_path, tag::ItemKey};
use serde::{Deserialize, Serialize};
use std::{
    collections::{HashMap, HashSet},
    fs::{self, File, FileTimes, OpenOptions},
    io::{Read, Write},
    path::{Component, Path, PathBuf},
};
use walkdir::WalkDir;

const UNKNOWN_GENRE: &str = "Unknown";
const COPY_BUFFER_SIZE: usize = 1024 * 1024;

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
pub struct Album {
    pub name: String,
    pub path: String,
    pub genres: Vec<String>,
    pub mp3_count: usize,
}

#[derive(Debug, Deserialize, PartialEq, Serialize)]
pub struct ScanResult {
    pub albums: Vec<Album>,
    pub warnings: Vec<String>,
}

#[derive(Clone, Debug, Serialize)]
pub struct CopyProgress {
    pub album_index: usize,
    pub album_count: usize,
    pub album_name: String,
}

pub fn scan_albums(root: &str) -> Result<ScanResult, String> {
    let root = root.trim();
    if root.is_empty() {
        return Err("Choose a music library folder.".into());
    }

    let root_path = Path::new(root);
    let root_metadata =
        fs::metadata(root_path).map_err(|error| format!("Open music library: {error}"))?;
    if !root_metadata.is_dir() {
        return Err(format!("Music library path is not a folder: {root}"));
    }

    let entries =
        fs::read_dir(root_path).map_err(|error| format!("Read music library: {error}"))?;
    let mut result = ScanResult {
        albums: Vec::new(),
        warnings: Vec::new(),
    };

    for entry in entries {
        let entry = entry.map_err(|error| format!("Read music library entry: {error}"))?;
        if !entry
            .file_type()
            .map_err(|error| format!("Inspect {}: {error}", entry.path().display()))?
            .is_dir()
        {
            continue;
        }

        let album_path = entry.path();
        let name = entry.file_name().to_string_lossy().into_owned();
        let mut genre_values = HashMap::<String, String>::new();
        let mut mp3_count = 0;

        for item in WalkDir::new(&album_path) {
            let item = item.map_err(|error| format!("Scan album {name:?}: {error}"))?;
            if !item.file_type().is_file()
                || !item.path().extension().is_some_and(|extension| {
                    extension.to_string_lossy().eq_ignore_ascii_case("mp3")
                })
            {
                continue;
            }

            mp3_count += 1;
            match read_from_path(item.path()) {
                Ok(tagged_file) => {
                    if let Some(genre) = tagged_file
                        .tags()
                        .iter()
                        .find_map(|tag| tag.get_string(&ItemKey::Genre))
                        .map(str::trim)
                        .filter(|genre| !genre.is_empty())
                    {
                        genre_values
                            .entry(genre.to_lowercase())
                            .or_insert_with(|| genre.to_owned());
                    }
                }
                Err(error) => result.warnings.push(format!(
                    "Could not read tags from {}: {error}",
                    item.path().display()
                )),
            }
        }

        if mp3_count == 0 {
            continue;
        }
        let mut genres = genre_values.into_values().collect::<Vec<_>>();
        if genres.is_empty() {
            genres.push(UNKNOWN_GENRE.into());
        }
        genres.sort_by_key(|genre| genre.to_lowercase());
        result.albums.push(Album {
            name,
            path: album_path.to_string_lossy().into_owned(),
            genres,
            mp3_count,
        });
    }

    result.albums.sort_by_key(|album| album.name.to_lowercase());
    Ok(result)
}

pub fn existing_album_folders(destination: &str, albums: &[Album]) -> Result<Vec<String>, String> {
    let mut existing = Vec::new();
    for album in albums {
        let path = Path::new(destination).join(&album.name);
        match fs::symlink_metadata(&path) {
            Ok(_) => existing.push(album.name.clone()),
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
            Err(error) => {
                return Err(format!("Check destination {}: {error}", path.display()));
            }
        }
    }
    Ok(existing)
}

pub fn copy_albums(
    destination: &str,
    albums: &[Album],
    cancelled: impl Fn() -> bool,
    mut progress: impl FnMut(CopyProgress) -> Result<(), String>,
) -> Result<(), String> {
    if albums.is_empty() {
        return Err("Select at least one album to copy.".into());
    }
    let destination = destination.trim();
    if destination.is_empty() {
        return Err("Choose a USB drive destination folder.".into());
    }

    let destination_root = absolute_clean(Path::new(destination))?;
    let mut targets = HashSet::new();
    let mut plans = Vec::with_capacity(albums.len());
    for album in albums {
        if cancelled() {
            return Err("Copy cancelled.".into());
        }
        let name = Path::new(&album.name);
        if album.name.is_empty()
            || name.components().count() != 1
            || !matches!(name.components().next(), Some(Component::Normal(_)))
        {
            return Err(format!("Invalid album folder name {:?}", album.name));
        }

        let source = absolute_clean(Path::new(&album.path))?;
        if !fs::metadata(&source)
            .map_err(|error| format!("Open album {:?}: {error}", album.name))?
            .is_dir()
        {
            return Err(format!("Album path is not a folder: {}", source.display()));
        }

        let target = destination_root.join(name);
        if paths_overlap(&source, &target) {
            return Err(format!(
                "Destination for album {:?} overlaps its source folder.",
                album.name
            ));
        }
        if !targets.insert(path_key(&target)) {
            return Err(format!("Duplicate album destination for {:?}", album.name));
        }
        plans.push((album.name.clone(), source, target));
    }

    fs::create_dir_all(&destination_root)
        .map_err(|error| format!("Create destination folder: {error}"))?;
    for (index, (name, source, target)) in plans.iter().enumerate() {
        if cancelled() {
            return Err("Copy cancelled.".into());
        }
        progress(CopyProgress {
            album_index: index + 1,
            album_count: plans.len(),
            album_name: name.clone(),
        })?;
        copy_directory(source, target, &cancelled)?;
    }
    Ok(())
}

fn copy_directory(
    source: &Path,
    destination: &Path,
    cancelled: &impl Fn() -> bool,
) -> Result<(), String> {
    for item in WalkDir::new(source) {
        if cancelled() {
            return Err("Copy cancelled.".into());
        }
        let item = item.map_err(|error| format!("Walk source folder: {error}"))?;
        let relative = item
            .path()
            .strip_prefix(source)
            .map_err(|error| format!("Resolve relative path: {error}"))?;
        let target = destination.join(relative);
        let file_type = item.file_type();

        if file_type.is_symlink() {
            return Err(format!(
                "Symbolic links are not supported: {}",
                item.path().display()
            ));
        }
        ensure_destination_path_is_safe(destination, &target)?;
        if file_type.is_dir() {
            fs::create_dir_all(&target)
                .map_err(|error| format!("Create folder {}: {error}", target.display()))?;
            continue;
        }
        if !file_type.is_file() {
            return Err(format!("Unsupported file type: {}", item.path().display()));
        }

        copy_file(item.path(), &target, cancelled)?;
    }
    Ok(())
}

fn ensure_destination_path_is_safe(album_root: &Path, target: &Path) -> Result<(), String> {
    let relative = target
        .strip_prefix(album_root)
        .map_err(|error| format!("Resolve destination path: {error}"))?;
    let mut path = album_root.to_path_buf();
    for component in std::iter::once(Component::CurDir).chain(relative.components()) {
        if component != Component::CurDir {
            path.push(component.as_os_str());
        }
        match fs::symlink_metadata(&path) {
            Ok(metadata) if metadata.file_type().is_symlink() => {
                return Err(format!(
                    "Destination contains a symbolic link: {}",
                    path.display()
                ));
            }
            Ok(_) => {}
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => break,
            Err(error) => {
                return Err(format!("Inspect destination {}: {error}", path.display()));
            }
        }
    }
    Ok(())
}

fn copy_file(
    source: &Path,
    destination: &Path,
    cancelled: &impl Fn() -> bool,
) -> Result<(), String> {
    let mut input = File::open(source)
        .map_err(|error| format!("Open source file {}: {error}", source.display()))?;
    if let Some(parent) = destination.parent() {
        fs::create_dir_all(parent)
            .map_err(|error| format!("Create folder {}: {error}", parent.display()))?;
    }
    let mut output = OpenOptions::new()
        .create(true)
        .truncate(true)
        .write(true)
        .open(destination)
        .map_err(|error| format!("Create destination file {}: {error}", destination.display()))?;
    let source_metadata = input
        .metadata()
        .map_err(|error| format!("Read source metadata: {error}"))?;
    let mut buffer = vec![0; COPY_BUFFER_SIZE];
    loop {
        if cancelled() {
            return Err("Copy cancelled.".into());
        }
        let count = input
            .read(&mut buffer)
            .map_err(|error| format!("Read {}: {error}", source.display()))?;
        if count == 0 {
            break;
        }
        output
            .write_all(&buffer[..count])
            .map_err(|error| format!("Write {}: {error}", destination.display()))?;
    }
    output
        .flush()
        .map_err(|error| format!("Flush {}: {error}", destination.display()))?;
    output
        .set_times(
            FileTimes::new().set_modified(
                source_metadata
                    .modified()
                    .map_err(|error| format!("Read source timestamp: {error}"))?,
            ),
        )
        .map_err(|error| format!("Set file timestamp on {}: {error}", destination.display()))?;
    output
        .set_permissions(source_metadata.permissions())
        .map_err(|error| format!("Set permissions on {}: {error}", destination.display()))?;
    Ok(())
}

fn absolute_clean(path: &Path) -> Result<PathBuf, String> {
    let absolute = if path.is_absolute() {
        path.to_path_buf()
    } else {
        std::env::current_dir()
            .map_err(|error| format!("Resolve current directory: {error}"))?
            .join(path)
    };
    let mut normalized = PathBuf::new();
    for component in absolute.components() {
        match component {
            Component::CurDir => {}
            Component::ParentDir => {
                normalized.pop();
            }
            other => normalized.push(other.as_os_str()),
        }
    }
    Ok(normalized)
}

fn paths_overlap(first: &Path, second: &Path) -> bool {
    path_is_within(first, second) || path_is_within(second, first)
}

fn path_is_within(parent: &Path, child: &Path) -> bool {
    let parent = parent.components().collect::<Vec<_>>();
    let child = child.components().collect::<Vec<_>>();
    child.len() >= parent.len()
        && parent
            .iter()
            .zip(child.iter())
            .all(|(left, right)| component_eq(left, right))
}

fn component_eq(first: &Component<'_>, second: &Component<'_>) -> bool {
    if cfg!(windows) {
        first
            .as_os_str()
            .to_string_lossy()
            .eq_ignore_ascii_case(&second.as_os_str().to_string_lossy())
    } else {
        first == second
    }
}

fn path_key(path: &Path) -> String {
    let path = path.to_string_lossy();
    if cfg!(windows) {
        path.to_lowercase()
    } else {
        path.into_owned()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::{fs, path::PathBuf};

    fn test_album(path: PathBuf, name: &str) -> Album {
        Album {
            name: name.into(),
            path: path.to_string_lossy().into_owned(),
            genres: vec![UNKNOWN_GENRE.into()],
            mp3_count: 1,
        }
    }

    #[test]
    fn scan_finds_immediate_album_folders_and_unknown_genre() {
        let library = tempfile::tempdir().unwrap();
        let track = library
            .path()
            .join("Artist - Album")
            .join("Disc 1")
            .join("track.mp3");
        fs::create_dir_all(track.parent().unwrap()).unwrap();
        fs::write(&track, b"not an ID3 file").unwrap();
        fs::write(library.path().join("ignored.mp3"), b"ignored").unwrap();

        let result = scan_albums(library.path().to_str().unwrap()).unwrap();
        assert_eq!(result.albums.len(), 1);
        assert_eq!(result.albums[0].name, "Artist - Album");
        assert_eq!(result.albums[0].mp3_count, 1);
        assert_eq!(result.albums[0].genres, [UNKNOWN_GENRE]);
        assert_eq!(result.warnings.len(), 1);
    }

    #[test]
    fn copy_preserves_nested_files_and_rejects_overlapping_destinations() {
        let source_root = tempfile::tempdir().unwrap();
        let album_path = source_root.path().join("Album");
        let track = album_path.join("Disc 1").join("track.mp3");
        fs::create_dir_all(track.parent().unwrap()).unwrap();
        fs::write(&track, b"track data").unwrap();
        let destination = tempfile::tempdir().unwrap();
        let album = test_album(album_path.clone(), "Album");

        copy_albums(
            destination.path().to_str().unwrap(),
            std::slice::from_ref(&album),
            || false,
            |_| Ok(()),
        )
        .unwrap();
        assert_eq!(
            fs::read(destination.path().join("Album/Disc 1/track.mp3")).unwrap(),
            b"track data"
        );

        let overlapping_destination = album_path.join("USB");
        assert!(
            copy_albums(
                overlapping_destination.to_str().unwrap(),
                &[album],
                || false,
                |_| Ok(()),
            )
            .is_err()
        );
        assert!(!overlapping_destination.exists());
    }

    #[test]
    fn cancelled_copy_does_not_create_destination() {
        let source_root = tempfile::tempdir().unwrap();
        let album_path = source_root.path().join("Album");
        fs::create_dir(&album_path).unwrap();
        let destination_root = tempfile::tempdir().unwrap();
        let destination = destination_root.path().join("USB");

        assert!(
            copy_albums(
                destination.to_str().unwrap(),
                &[test_album(album_path, "Album")],
                || true,
                |_| Ok(()),
            )
            .is_err()
        );
        assert!(!destination.exists());
    }
}
