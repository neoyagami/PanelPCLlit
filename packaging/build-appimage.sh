#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd "$(dirname "$0")/.." && pwd)"
architecture="${APPIMAGE_ARCH:-$(uname -m)}"
case "$architecture" in
  amd64) architecture=x86_64 ;;
  arm64) architecture=aarch64 ;;
  x86_64|aarch64) ;;
  *) echo "Unsupported AppImage architecture: $architecture" >&2; exit 2 ;;
esac

build_root="${PANELPC_APPIMAGE_BUILD_DIR:-$project_dir/build/appimage-$architecture}"
app_dir="$build_root/AppDir"
output="${PANELPC_APPIMAGE_OUTPUT:-$project_dir/dist/PanelPC-$architecture.AppImage}"
linuxdeploy_command="${LINUXDEPLOY:-linuxdeploy-$architecture.AppImage}"

if ! command -v "$linuxdeploy_command" >/dev/null 2>&1 && [[ ! -x "$linuxdeploy_command" ]]; then
  echo "linuxdeploy was not found. Set LINUXDEPLOY to its executable path." >&2
  exit 1
fi

rm -rf "$app_dir"
mkdir -p "$app_dir/usr/bin" "$app_dir/usr/share/panelpc" "$(dirname "$output")"

if [[ -n "${PANELPC_QT_BINARY:-}" ]]; then
  source_binary="$PANELPC_QT_BINARY"
else
  source_binary="$build_root/panelpc-qt"
  mkdir -p "$build_root"
  # AppImage builds may run in a container where the mounted checkout is not a
  # Git safe directory. VCS metadata is not needed in the packaged binary, so
  # disable stamping explicitly instead of depending on the builder's Git
  # configuration.
  (cd "$project_dir" && go build -buildvcs=false -tags qt -trimpath -ldflags="-s -w" -o "$source_binary" ./cmd/panelpc-qt)
fi
if [[ ! -x "$source_binary" ]]; then
  echo "Missing Qt executable: $source_binary" >&2
  exit 1
fi

install -m 0755 "$source_binary" "$app_dir/usr/bin/panelpc-qt"
install -m 0755 "$project_dir/packaging/AppRun" "$app_dir/AppRun"
install -m 0755 "$project_dir/packaging/install-appimage-user.sh" "$app_dir/usr/share/panelpc/install-appimage-user.sh"
install -m 0644 "$project_dir/packaging/panelpc.desktop" "$app_dir/panelpc.desktop"
install -m 0644 "$project_dir/assets/panelpc.svg" "$app_dir/panelpc.svg"
install -m 0644 "$project_dir/LICENSE" "$app_dir/usr/share/panelpc/LICENSE"

export APPIMAGE_EXTRACT_AND_RUN=1
export DISABLE_COPYRIGHT_FILES_DEPLOYMENT=1
# linuxdeploy bundles an older strip implementation that cannot parse some
# newer ELF features (for example Fedora's .relr.dyn sections). The Go binary
# is already stripped by the linker, and Qt libraries must remain compatible
# with the oldest supported build environment, so disabling the extra pass is
# both safer and reproducible across local and CI builders.
export NO_STRIP="${NO_STRIP:-1}"
export EXTRA_PLATFORM_PLUGINS="${EXTRA_PLATFORM_PLUGINS:-libqoffscreen.so;libqwayland-egl.so;libqwayland-generic.so}"
export LDAI_OUTPUT="$output.new"
# Older appimage output plugins use OUTPUT; current releases prefer
# LDAI_OUTPUT. Export both so local and CI builds behave identically.
export OUTPUT="$LDAI_OUTPUT"
"$linuxdeploy_command" \
  --appdir "$app_dir" \
  --executable "$app_dir/usr/bin/panelpc-qt" \
  --desktop-file "$project_dir/packaging/panelpc.desktop" \
  --icon-file "$project_dir/assets/panelpc.svg" \
  --custom-apprun "$project_dir/packaging/AppRun" \
  --plugin qt \
  --output appimage
mv -f -- "$output.new" "$output"
(cd "$(dirname "$output")" && sha256sum "$(basename "$output")" > "$(basename "$output").sha256")
echo "Created $output"
