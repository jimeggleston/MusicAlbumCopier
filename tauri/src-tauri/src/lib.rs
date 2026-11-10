use music_album_copier_core::{self as library, Album, CopyProgress, ScanResult};
use std::sync::{
    Arc, Mutex,
    atomic::{AtomicBool, Ordering},
};
use tauri::{AppHandle, Emitter, State};

#[derive(Default)]
struct CopyState {
    active: Mutex<Option<Arc<AtomicBool>>>,
}

#[tauri::command]
async fn scan_albums(root: String) -> Result<ScanResult, String> {
    match tauri::async_runtime::spawn_blocking(move || library::scan_albums(&root)).await {
        Ok(result) => result,
        Err(error) => Err(format!("Album scan task failed: {error}")),
    }
}

#[tauri::command]
fn existing_album_folders(destination: String, albums: Vec<Album>) -> Result<Vec<String>, String> {
    library::existing_album_folders(&destination, &albums)
}

#[tauri::command]
async fn copy_albums(
    app: AppHandle,
    state: State<'_, CopyState>,
    destination: String,
    albums: Vec<Album>,
) -> Result<(), String> {
    let cancellation = Arc::new(AtomicBool::new(false));
    {
        let mut active = state
            .active
            .lock()
            .map_err(|_| "Copy state is unavailable.".to_owned())?;
        if active.is_some() {
            return Err("A copy operation is already in progress.".into());
        }
        *active = Some(cancellation.clone());
    }

    let task_cancellation = cancellation.clone();
    let result = match tauri::async_runtime::spawn_blocking(move || {
        library::copy_albums(
            &destination,
            &albums,
            || task_cancellation.load(Ordering::Relaxed),
            |progress: CopyProgress| {
                app.emit("copy-progress", progress)
                    .map_err(|error| format!("Report copy progress: {error}"))
            },
        )
    })
    .await
    {
        Ok(result) => result,
        Err(error) => Err(format!("Copy task failed: {error}")),
    };

    let mut active = state
        .active
        .lock()
        .map_err(|_| "Copy state is unavailable.".to_owned())?;
    if active
        .as_ref()
        .is_some_and(|active| Arc::ptr_eq(active, &cancellation))
    {
        *active = None;
    }
    result
}

#[tauri::command]
fn cancel_copy(state: State<'_, CopyState>) -> Result<(), String> {
    if let Some(active) = state
        .active
        .lock()
        .map_err(|_| "Copy state is unavailable.".to_owned())?
        .as_ref()
    {
        active.store(true, Ordering::Relaxed);
    }
    Ok(())
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .manage(CopyState::default())
        .invoke_handler(tauri::generate_handler![
            scan_albums,
            existing_album_folders,
            copy_albums,
            cancel_copy
        ])
        .run(tauri::generate_context!())
        .expect("error while running Music Album Copier");
}
