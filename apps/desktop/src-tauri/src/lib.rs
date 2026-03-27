use tauri::LogicalSize;

#[tauri::command]
fn resize_for_route(window: tauri::Window, route: String) {
    let is_login = route == "/login" || route == "/";
    if is_login {
        let _ = window.set_min_size(None::<tauri::Size>);
        let _ = window.set_size(tauri::Size::Logical(LogicalSize::new(480.0, 700.0)));
    } else {
        let _ = window.set_size(tauri::Size::Logical(LogicalSize::new(1280.0, 800.0)));
        let _ = window.set_min_size(Some(tauri::Size::Logical(LogicalSize::new(960.0, 600.0))));
    }
    let _ = window.center();
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .invoke_handler(tauri::generate_handler![resize_for_route])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
