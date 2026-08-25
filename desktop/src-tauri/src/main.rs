#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

//! OpenAIDE 桌面壳 —— Tauri v2。
//! 职责：定位/拉起 `openaide serve` 后端进程 → 等健康检查通过 → 打开指向
//! 本地 WebUI 的窗口；应用退出时回收后端进程。界面本身完全由 WebUI 承担。

use std::io::{Read, Write};
use std::net::{TcpListener, TcpStream};
use std::path::PathBuf;
use std::process::{Child, Command, Stdio};
use std::sync::Mutex;
use std::time::{Duration, Instant};

use tauri::{Manager, RunEvent, WebviewUrl, WebviewWindowBuilder};

struct Backend(Mutex<Option<Child>>);

fn resolve_openaide_bin() -> PathBuf {
    // 优先级：OPENAIDE_BIN > 可执行文件同目录 > PATH（交给系统解析）
    if let Ok(p) = std::env::var("OPENAIDE_BIN") {
        let pb = PathBuf::from(p);
        if pb.exists() {
            return pb;
        }
    }
    if let Ok(exe) = std::env::current_exe() {
        if let Some(dir) = exe.parent() {
            for name in ["openaide.exe", "openaide"] {
                let cand = dir.join(name);
                if cand.exists() {
                    return cand;
                }
            }
        }
    }
    PathBuf::from("openaide")
}

fn pick_port() -> u16 {
    TcpListener::bind("127.0.0.1:0")
        .expect("bind 127.0.0.1:0")
        .local_addr()
        .unwrap()
        .port()
}

/// 依赖-free 健康检查：裸 TCP 写 HTTP GET，读到 `"ok"` 即认为就绪
fn healthy(port: u16) -> bool {
    if let Ok(mut s) = TcpStream::connect(("127.0.0.1", port)) {
        let _ = s.set_read_timeout(Some(Duration::from_millis(800)));
        let req = format!("GET /health HTTP/1.1\r\nHost: 127.0.0.1:{port}\r\nConnection: close\r\n\r\n");
        if s.write_all(req.as_bytes()).is_ok() {
            let mut buf = String::new();
            let _ = s.read_to_string(&mut buf);
            return buf.contains("\"ok\"");
        }
    }
    false
}

fn wait_healthy(port: u16, timeout: Duration) -> bool {
    let start = Instant::now();
    while start.elapsed() < timeout {
        if healthy(port) {
            return true;
        }
        std::thread::sleep(Duration::from_millis(300));
    }
    false
}

fn main() {
    let port = pick_port();
    let bin = resolve_openaide_bin();

    // 拉起后端（serve），输出静默——错误经健康检查超时暴露在 UI 提示里
    let child = Command::new(&bin)
        .args(["serve", "--port", &port.to_string()])
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn();

    let child = match child {
        Ok(c) => c,
        Err(e) => {
            eprintln!(
                "[openaide-desktop] failed to launch backend '{bin:?}': {e}\n\
                 Place the openaide binary next to the desktop app, set OPENAIDE_BIN, \
                 or install the Node edition (npm i -g @openaide/cli)."
            );
            // 无后端也要有窗口：打开一个内置说明页，避免闪退
            tauri::Builder::default()
                .setup(|app| {
                    WebviewWindowBuilder::new(
                        app,
                        "main",
                        WebviewUrl::External("http://127.0.0.1:9/no-backend".parse().unwrap()),
                    )
                    .title("OpenAIDE — backend not found")
                    .inner_size(900., 600.)
                    .build()?;
                    Ok(())
                })
                .run(tauri::generate_context!())
                .expect("error while running tauri application");
            return;
        }
    };

    let state = Backend(Mutex::new(Some(child)));
    let port_for_wait = port;

    tauri::Builder::default()
        .manage(state)
        .setup(move |app| {
            let handle = app.handle().clone();
            std::thread::spawn(move || {
                if !wait_healthy(port_for_wait, Duration::from_secs(20)) {
                    eprintln!("[openaide-desktop] backend health check timed out");
                }
                let url = format!("http://127.0.0.1:{port_for_wait}");
                WebviewWindowBuilder::new(
                    &handle,
                    "main",
                    WebviewUrl::External(url.parse().expect("valid url")),
                )
                .title("OpenAIDE")
                .inner_size(1200., 820.)
                .min_inner_size(860., 560.)
                .build()
                .expect("failed to create main window");
            });
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("error while building tauri application")
        .run(move |_app, event| {
            if let RunEvent::Exit = event {
                if let Some(child) = _app.state::<Backend>().0.lock().unwrap().as_mut() {
                    let _ = child.kill(); // 退出时回收后端进程
                }
            }
        });
}
