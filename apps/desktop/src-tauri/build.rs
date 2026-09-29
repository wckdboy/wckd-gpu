fn main() {
    tauri_build::try_build(
        tauri_build::Attributes::new().app_manifest(
            tauri_build::AppManifest::new().commands(&[
                "config_check",
                "list_offers",
                "start_session",
                "session_status",
                "stop_session",
                "list_presets",
                "list_projects",
                "add_project",
                "open_external",
            ]),
        ),
    )
    .expect("failed to run tauri-build");
}
