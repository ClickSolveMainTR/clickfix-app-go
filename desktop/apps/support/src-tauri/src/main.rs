// Windows sürüm derlemesinde ek konsol penceresi açılmasın.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    clickfix_support_lib::run();
}
