#!/usr/bin/env bash
set -euo pipefail

if [[ "$EUID" -eq 0 ]]; then
  echo "Install the AppImage as your regular desktop user, without sudo." >&2
  exit 2
fi

mode="${1:-}"
source_appimage="${2:-}"
desktop_template="${3:-}"
icon_source="${4:-}"
shift $(( $# < 4 ? $# : 4 ))

data_home="${XDG_DATA_HOME:-$HOME/.local/share}"
config_home="${XDG_CONFIG_HOME:-$HOME/.config}"
install_dir="$data_home/panelpc"
installed_appimage="$install_dir/PanelPC.AppImage"
applications_dir="$data_home/applications"
icons_dir="$data_home/icons/hicolor/scalable/apps"
desktop_file="$applications_dir/panelpc.desktop"
autostart_file="$config_home/autostart/panelpc.desktop"

refresh_desktop_database() {
  if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database "$applications_dir" >/dev/null 2>&1 || true
  fi
}

write_desktop_entry() {
  local destination="$1"
  local extra_arguments="${2:-}"
  local escaped_exec
  escaped_exec="${installed_appimage//\\/\\\\}"
  escaped_exec="${escaped_exec//\"/\\\"}"
  escaped_exec="${escaped_exec//&/\\&}"
  escaped_exec="${escaped_exec//|/\\|}"
  sed "s|^Exec=.*|Exec=\"$escaped_exec\"$extra_arguments|" "$desktop_template" > "$destination.new"
  chmod 0644 "$destination.new"
  mv -f -- "$destination.new" "$destination"
}

if [[ "$mode" == uninstall ]]; then
  rm -f -- "$desktop_file" "$autostart_file" "$icons_dir/panelpc.svg" "$installed_appimage"
  rmdir -- "$install_dir" 2>/dev/null || true
  refresh_desktop_database
  echo "PanelPC was removed from this user account. Personal configuration was kept."
  exit 0
fi

if [[ "$mode" == autostart-off ]]; then
  rm -f -- "$autostart_file"
  echo "PanelPC autostart was disabled."
  exit 0
fi

if [[ "$mode" == autostart-on ]]; then
  if [[ ! -x "$installed_appimage" ]] || [[ ! -f "$desktop_template" ]]; then
    echo "Install PanelPC first with --install-user." >&2
    exit 1
  fi
  mkdir -p "$(dirname "$autostart_file")"
  write_desktop_entry "$autostart_file" " --background"
  echo "PanelPC will start with the desktop session."
  exit 0
fi

if [[ "$mode" != install ]] || [[ ! -f "$source_appimage" ]] || \
   [[ ! -f "$desktop_template" ]] || [[ ! -f "$icon_source" ]]; then
  echo "The AppImage installation files are incomplete." >&2
  exit 1
fi

autostart=false
for argument in "$@"; do
  case "$argument" in
    --autostart) autostart=true ;;
    *) echo "Unknown install option: $argument" >&2; exit 2 ;;
  esac
done

mkdir -p "$install_dir" "$applications_dir" "$icons_dir"
if [[ "$(readlink -f "$source_appimage")" != "$(readlink -f "$installed_appimage" 2>/dev/null || true)" ]]; then
  install -m 0755 "$source_appimage" "$installed_appimage.new"
  mv -f -- "$installed_appimage.new" "$installed_appimage"
fi
install -m 0644 "$icon_source" "$icons_dir/panelpc.svg"
write_desktop_entry "$desktop_file"

if [[ "$autostart" == true ]]; then
  mkdir -p "$(dirname "$autostart_file")"
  write_desktop_entry "$autostart_file" " --background"
fi

refresh_desktop_database
if [[ "$autostart" == true ]]; then
  echo "PanelPC was installed for this user and will start with the desktop session."
else
  echo "PanelPC was installed for this user and added to the application menu."
fi
