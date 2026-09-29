use std::io::Read;
use std::process::{Command, Stdio};
use std::time::{Duration, Instant};

use serde::{Deserialize, Serialize};
use tauri_plugin_opener::OpenerExt;

const QUICK: Duration = Duration::from_secs(90);
const SLOW: Duration = Duration::from_secs(6 * 60);

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct BridgeOpts {
    bin: String,
    config_path: String,
    work_dir: String,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct CmdOutput {
    code: i32,
    stdout: String,
    stderr: String,
}

#[tauri::command]
fn config_check(opts: BridgeOpts) -> Result<CmdOutput, String> {
    spawn_wckd(&opts, vec!["config".into(), "check".into(), "--json".into()], QUICK)
}

#[tauri::command]
fn list_offers(opts: BridgeOpts, preset: String, hours: f64, limit: u32) -> Result<CmdOutput, String> {
    require_slug(&preset, "preset id")?;
    require_hours(hours)?;
    require_limit(limit)?;
    spawn_wckd(
        &opts,
        vec![
            "offers".into(),
            "--json".into(),
            "--preset".into(),
            preset,
            "--hours".into(),
            format_hours(hours),
            "--limit".into(),
            limit.to_string(),
        ],
        QUICK,
    )
}

#[tauri::command]
fn start_session(
    opts: BridgeOpts,
    project: String,
    preset: String,
    hours: f64,
    offer_id: String,
    dry_run: bool,
) -> Result<CmdOutput, String> {
    require_slug(&project, "project id")?;
    require_slug(&preset, "preset id")?;
    require_hours(hours)?;
    require_offer_id(&offer_id)?;
    let mut args = vec![
        "start".into(),
        "--json".into(),
        "--hours".into(),
        format_hours(hours),
        "--project".into(),
        project,
        "--preset".into(),
        preset,
    ];
    if !offer_id.is_empty() {
        args.push("--offer".into());
        args.push(offer_id);
    }
    if dry_run {
        args.push("--dry-run".into());
    }
    spawn_wckd(&opts, args, SLOW)
}

#[tauri::command]
fn session_status(opts: BridgeOpts, session_id: String) -> Result<CmdOutput, String> {
    require_session_id(&session_id)?;
    let mut args = vec!["status".into(), "--json".into()];
    if !session_id.is_empty() {
        args.push(session_id);
    }
    spawn_wckd(&opts, args, QUICK)
}

#[tauri::command]
fn stop_session(opts: BridgeOpts, session_id: String, force: bool) -> Result<CmdOutput, String> {
    require_session_id(&session_id)?;
    let mut args = vec!["stop".into(), "--json".into()];
    if !session_id.is_empty() {
        args.push(session_id);
    }
    if force {
        args.push("--force".into());
    }
    spawn_wckd(&opts, args, SLOW)
}

#[tauri::command]
fn list_presets(opts: BridgeOpts) -> Result<CmdOutput, String> {
    spawn_wckd(&opts, vec!["presets".into(), "--json".into()], QUICK)
}

#[tauri::command]
fn list_projects(opts: BridgeOpts) -> Result<CmdOutput, String> {
    spawn_wckd(&opts, vec!["projects".into(), "--json".into()], QUICK)
}

#[tauri::command]
fn add_project(opts: BridgeOpts, id: String) -> Result<CmdOutput, String> {
    require_slug(&id, "project id")?;
    spawn_wckd(
        &opts,
        vec!["projects".into(), "add".into(), id, "--json".into()],
        QUICK,
    )
}

#[tauri::command]
fn open_external(app: tauri::AppHandle, url: String) -> Result<(), String> {
    require_http_url(&url)?;
    app.opener()
        .open_url(url, None::<&str>)
        .map_err(|err| err.to_string())
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .invoke_handler(tauri::generate_handler![
            config_check,
            list_offers,
            start_session,
            session_status,
            stop_session,
            list_presets,
            list_projects,
            add_project,
            open_external
        ])
        .run(tauri::generate_context!())
        .expect("error while running wckd-gpu");
}

fn spawn_wckd(opts: &BridgeOpts, args: Vec<String>, timeout: Duration) -> Result<CmdOutput, String> {
    let bin = normalize_bin(&opts.bin)?;
    let mut cmd = Command::new(&bin);
    cmd.stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    if let Some(dir) = normalize_dir(&opts.work_dir)? {
        cmd.current_dir(dir);
    }
    let mut full = Vec::new();
    if let Some(path) = normalize_file(&opts.config_path)? {
        full.push("--config".to_string());
        full.push(path);
    }
    full.extend(args);
    cmd.args(&full);
    let mut child = cmd
        .spawn()
        .map_err(|err| format!("could not start {bin}: {err}"))?;
    let started = Instant::now();
    loop {
        match child.try_wait() {
            Ok(Some(status)) => {
                return Ok(CmdOutput {
                    code: status.code().unwrap_or(-1),
                    stdout: child
                        .stdout
                        .as_mut()
                        .map(read_to_string)
                        .unwrap_or_default(),
                    stderr: child
                        .stderr
                        .as_mut()
                        .map(read_to_string)
                        .unwrap_or_default(),
                });
            }
            Ok(None) if started.elapsed() > timeout => {
                let _ = child.kill();
                let _ = child.wait();
                return Err(format!("wckd timed out after {}s", timeout.as_secs()));
            }
            Ok(None) => std::thread::sleep(Duration::from_millis(200)),
            Err(err) => return Err(format!("wckd: {err}")),
        }
    }
}

fn read_to_string(pipe: &mut impl Read) -> String {
    let mut buf = String::new();
    let _ = pipe.read_to_string(&mut buf);
    buf
}

fn normalize_bin(bin: &str) -> Result<String, String> {
    let trimmed = bin.trim();
    let bin = if trimmed.is_empty() { "wckd" } else { trimmed };
    if bin.len() > 4096 || bin.chars().any(|c| c.is_control()) {
        return Err("wckd binary path is invalid".into());
    }
    Ok(bin.to_string())
}

fn normalize_dir(dir: &str) -> Result<Option<String>, String> {
    let dir = dir.trim();
    if dir.is_empty() {
        return Ok(None);
    }
    if dir.chars().any(|c| c.is_control()) {
        return Err("working directory is invalid".into());
    }
    let meta = std::fs::metadata(dir).map_err(|_| format!("working directory does not exist: {dir}"))?;
    if !meta.is_dir() {
        return Err("working directory is not a directory".into());
    }
    Ok(Some(dir.to_string()))
}

fn normalize_file(path: &str) -> Result<Option<String>, String> {
    let path = path.trim();
    if path.is_empty() {
        return Ok(None);
    }
    if path.chars().any(|c| c.is_control()) {
        return Err("config path is invalid".into());
    }
    let meta = std::fs::metadata(path).map_err(|_| format!("config file does not exist: {path}"))?;
    if !meta.is_file() {
        return Err("config path is not a file".into());
    }
    Ok(Some(path.to_string()))
}

fn require_slug(value: &str, what: &str) -> Result<(), String> {
    let mut chars = value.chars();
    let Some(first) = chars.next() else {
        return Err(format!("{what} is invalid"));
    };
    if !first.is_ascii_alphanumeric() {
        return Err(format!("{what} is invalid"));
    }
    let mut len = 1;
    for c in chars {
        len += 1;
        if len > 64 || !(c.is_ascii_alphanumeric() || c == '-' || c == '_') {
            return Err(format!("{what} is invalid"));
        }
    }
    Ok(())
}

fn require_hours(hours: f64) -> Result<(), String> {
    if !hours.is_finite() || hours <= 0.0 || hours > 24.0 {
        return Err("hours must be within (0, 24]".into());
    }
    Ok(())
}

fn require_limit(limit: u32) -> Result<(), String> {
    if !(1..=100).contains(&limit) {
        return Err("limit must be 1..=100".into());
    }
    Ok(())
}

fn require_offer_id(id: &str) -> Result<(), String> {
    if id.is_empty() {
        return Ok(());
    }
    if id.len() > 300 || id.chars().any(|c| c.is_control()) || !id.starts_with("runpod:") {
        return Err("offer id is invalid".into());
    }
    Ok(())
}

fn require_session_id(id: &str) -> Result<(), String> {
    if id.is_empty() {
        return Ok(());
    }
    let Some(rest) = id.strip_prefix("sess_") else {
        return Err("session id is invalid".into());
    };
    if !(8..=32).contains(&rest.len())
        || !rest.chars().all(|c| c.is_ascii_hexdigit() && !c.is_ascii_uppercase())
    {
        return Err("session id is invalid".into());
    }
    Ok(())
}

fn require_http_url(url: &str) -> Result<(), String> {
    let lower = url.to_ascii_lowercase();
    if !(lower.starts_with("https://") || lower.starts_with("http://")) {
        return Err("only http(s) URLs can be opened".into());
    }
    if url.chars().any(|c| c.is_control() || c.is_whitespace()) {
        return Err("url is invalid".into());
    }
    Ok(())
}

fn format_hours(hours: f64) -> String {
    let raw = format!("{hours:.4}");
    raw.trim_end_matches('0').trim_end_matches('.').to_string()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn slugs_match_the_cli() {
        assert!(require_slug("hailuo-tests", "project id").is_ok());
        assert!(require_slug("../etc", "project id").is_err());
        assert!(require_slug("-nope", "project id").is_err());
        assert!(require_session_id("").is_ok());
        assert!(require_session_id("sess_abc123def456").is_ok());
        assert!(require_session_id("sess_ABC").is_err());
        assert!(require_offer_id("runpod:community:NVIDIA GeForce RTX 4090").is_ok());
        assert!(require_offer_id("vast:1").is_err());
        assert!(require_hours(0.0).is_err());
        assert!(require_hours(24.0).is_ok());
        assert!(require_http_url("https://pod.example/ui").is_ok());
        assert!(require_http_url("file:///etc/passwd").is_err());
        assert_eq!(format_hours(1.0), "1");
        assert_eq!(format_hours(1.5), "1.5");
    }
}
