#!/bin/sh
set -eu

usage() {
    cat <<'EOF'
Usage: ./install.sh [--user|--system] [--no-start]

  --user     Install under ~/.local (default and recommended).
             Works on Bazzite, Fedora Atomic, SteamOS, and traditional distros.
  --system   Install the binary under /usr/local on a traditional distro.
             The desktop integration remains a per-user systemd service.
  --no-start Install files without enabling or starting the service.

Do not run this installer with sudo or as root. In --system mode it requests
sudo only for the two system-wide file copies.
EOF
}

mode=user
start_service=1
for argument in "$@"; do
    case "$argument" in
        --user) mode=user ;;
        --system) mode=system ;;
        --no-start) start_service=0 ;;
        -h|--help) usage; exit 0 ;;
        *) printf 'Unknown option: %s\n' "$argument" >&2; usage >&2; exit 2 ;;
    esac
done

if [ "$(id -u)" -eq 0 ]; then
    printf '%s\n' 'Refusing to run as root. Run ./install.sh as your desktop user.' >&2
    exit 1
fi
current_user=$(id -un)

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
source_binary="$script_dir/panelpc"
service_template="$script_dir/panelpc.service"

if [ ! -f "$source_binary" ]; then
    printf 'Missing %s. Run this script from an extracted PanelPC release ZIP.\n' "$source_binary" >&2
    exit 1
fi
if [ ! -f "$service_template" ]; then
    printf 'Missing %s. The release archive may be incomplete.\n' "$service_template" >&2
    exit 1
fi

immutable=0
if [ -e /run/ostree-booted ]; then
    immutable=1
elif [ -r /etc/os-release ] && grep -Eqi '^(ID|ID_LIKE|VARIANT_ID)=.*(bazzite|silverblue|kinoite|atomic|steamos)' /etc/os-release; then
    immutable=1
fi

if [ "$mode" = system ] && [ "$immutable" -eq 1 ]; then
    printf '%s\n' 'This appears to be an immutable/stateless distro.' >&2
    printf '%s\n' 'Use the recommended rootless installation: ./install.sh --user' >&2
    exit 1
fi

unit_dir=${XDG_CONFIG_HOME:-"$HOME/.config"}/systemd/user
unit_file="$unit_dir/panelpc.service"
temporary_unit=$(mktemp "${TMPDIR:-/tmp}/panelpc-service.XXXXXX")
trap 'rm -f "$temporary_unit"' EXIT HUP INT TERM

if [ "$mode" = user ]; then
    binary_dir="$HOME/.local/bin"
    binary_path="$binary_dir/panelpc"
    install -d -m 0755 "$binary_dir" "$unit_dir"
    install -m 0755 "$source_binary" "$binary_path"
else
    binary_path=/usr/local/bin/panelpc
    command -v sudo >/dev/null 2>&1 || {
        printf '%s\n' '--system requires sudo for copying files to /usr/local.' >&2
        exit 1
    }
    sudo install -d -m 0755 /usr/local/bin
    sudo install -m 0755 "$source_binary" "$binary_path"
    install -d -m 0755 "$unit_dir"
fi

sed "s|@PANELPC_BIN@|$binary_path|g" "$service_template" > "$temporary_unit"
install -m 0644 "$temporary_unit" "$unit_file"

missing_tools=
for tool in wpctl pactl parec; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        missing_tools="$missing_tools $tool"
    fi
done

if [ "$start_service" -eq 1 ]; then
    if command -v systemctl >/dev/null 2>&1; then
        systemctl --user daemon-reload
        systemctl --user enable --now panelpc.service
        printf 'PanelPC installed and started for %s.\n' "$current_user"
    else
        printf '%s\n' 'systemctl was not found; start PanelPC manually:'
        printf '  %s\n' "$binary_path"
    fi
else
    printf 'PanelPC installed at %s (service not started).\n' "$binary_path"
fi

if [ -n "$missing_tools" ]; then
    printf 'Warning: these optional runtime tools were not found:%s\n' "$missing_tools" >&2
fi

printf '%s\n' 'No driver was installed and no udev configuration was changed.'
printf '%s\n' 'Open http://127.0.0.1:8765 after the service starts.'
