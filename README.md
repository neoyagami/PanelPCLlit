# PanelPC

PanelPC is a small native Linux controller for the PCPanel Lite/Mini with four clickable RGB knobs. It controls PipeWire/PulseAudio audio, device lighting, and OBS Studio through WebSocket 5 from an embedded local web interface.

![PanelPC web interface showing four side-by-side knob controls, RGB spectrum mode, and OBS integration](assets/interface.png)

## Motivation

PanelPC exists because the two available solutions we tested—the official application and a community alternative—did not work adequately on Linux. The observed problems included an impractical interface, poor PipeWire/PulseAudio channel selection, limited OBS integration, and, during one test, a system that became unresponsive when a knob was moved.

This project was built specifically for Linux and communicates directly with `hidraw`, PipeWire/PulseAudio, and OBS WebSocket. It does not capture keyboard or mouse input and does not depend on an application primarily designed for another operating system.

## Features

- Direct support for the four-knob PCPanel Lite/Mini, including clicks and RGB lighting.
- Volume and mute control for the default output or input.
- Selection of specific PipeWire/PulseAudio devices and application streams.
- OBS WebSocket 5 integration: volume, mute, scenes, recording, streaming, and numeric filter settings.
- Per-knob RGB colors, global brightness, and dial-position brightness tracking.
- **VU Toy** and **four-band spectrum analyzer** modes that turn the physical PCPanel into a real-time desktop audio visualizer.
- Shell actions for turns and clicks, with level variables, timeout, and configurable rate limiting.
- Complete profiles and profile switching from a physical knob click.
- Responsive, collapsible local web interface embedded in a single Go binary.
- Bounded queues and event coalescing to avoid blocking the desktop or flooding USB.

Exposing the panel as an OpenRGB device is considered a future enhancement and is not part of the current implementation.

## AI-assisted development

This project does not hide its use of artificial intelligence. The initial design, code, and documentation were developed with assistance from **OpenAI Codex**, under human direction. Functional decisions were specified and reviewed by the hardware owner, and the integrations were tested during development with a real PCPanel Mini, a PipeWire/PulseAudio session, and OBS Studio.

AI assistance is not a substitute for code review. Automated tests, Go's race detector, and static analysis are run before builds. Review the code and any configured commands before using it on another system.

## Why it should not freeze the desktop

- It reads only the PCPanel `hidraw` node; it never captures keyboard or mouse input.
- The HID queue has a fixed capacity of 32 events and never blocks the device reader.
- Knob turns provide absolute values, so only the latest value for each knob is retained every 50 ms.
- Only one external operation runs at a time, with at most one additional operation waiting.
- Every `wpctl` or `pactl` call times out after 1.5 seconds.
- Firmware announcements received during the first 400 ms are ignored as actions, preventing unexpected audio changes when the panel is connected.
- Initial knob positions are still displayed in the interface for diagnostics.
- RGB output uses a single-slot queue where a newer update replaces the pending one.
- The audio meter uses continuous capture and limits RGB writes to 5–30 updates per second.
- Turn commands have a configurable minimum interval and retain only the newest level.

## Building

PanelPC requires Go 1.23 or newer and has no third-party Go modules.

```bash
make test
make build
./build/panelpc
```

The interface opens at `http://127.0.0.1:8765`. To run without opening a browser:

```bash
./build/panelpc -no-browser
```

## Installing a release

Download the ZIP for your architecture from the GitHub Releases page, verify the published checksum, extract it, and inspect the installer before running it:

```bash
sha256sum -c SHA256SUMS
unzip panelpc-linux-amd64.zip
cd panelpc-linux-amd64
less install.sh
./install.sh --user
```

The rootless `--user` installation is the default and recommended mode everywhere. It places the binary in `~/.local/bin`, installs a systemd user unit, and starts PanelPC in the current desktop session.

### Bazzite and other immutable/stateless distributions

Use the rootless installation:

```bash
./install.sh --user
```

This mode is designed for Bazzite, Fedora Atomic desktops such as Silverblue and Kinoite, SteamOS, and similar systems. It writes only to the user's home directory, survives operating-system image updates, and does not use `rpm-ostree`, layering, or writable overlays.

### Traditional distributions

The same `--user` mode is recommended on Fedora, Ubuntu, Debian, Arch Linux, and other conventional distributions. An optional system-wide binary installation is available:

```bash
./install.sh --system
```

Do not run the installer itself with `sudo`. In `--system` mode it requests `sudo` only while copying the binary to `/usr/local/bin`; PanelPC still runs as a systemd user service so it can access the user's PipeWire session and OBS instance.

### No drivers or device-permission changes

PanelPC does **not** install a kernel module, USB driver, HID driver, or background input driver. There is no `udev` rule in this repository, and the installer does not create, edit, reload, or remove one. No `MODE="0666"`, custom group membership, or broad HID permission change is applied.

You do not need to install drivers or change `udev` configuration. PanelPC uses the device access already provided to the active desktop session. If a distribution reports a permission error, do not run PanelPC as root and do not grant global access to every `hidraw` device; open an issue with the distribution, desktop session, and device information instead.

## Audio

PanelPC uses standard tools already included with PipeWire-based Linux systems:

- `wpctl` for the default output and microphone.
- `pactl` for application streams. The selector does not depend on ephemeral PulseAudio indexes; it resolves the application name whenever the stream list is refreshed.
- `pactl` for specific outputs and inputs. The interface displays the friendly description while storing the stable `alsa_output...` or `alsa_input...` name.

Selectors are populated from the active PipeWire/PulseAudio session. Output monitor sources are excluded from the microphone list so they are not confused with real capture devices.

## Shell commands

A turn or click can execute a command through `/bin/sh -c`. The process receives:

- `LEVEL`, rounded from 0 to 100.
- `RAW_LEVEL`, the original value from 0 to 255.

Example:

```sh
/bin/echo "$LEVEL" > /tmp/panelpc-level
```

Turn actions support a minimum interval from 50 to 60000 ms. Every execution times out after five seconds, and PanelPC terminates the complete process group if the timeout is exceeded.

## OBS

In OBS, open **Tools → WebSocket Server Settings**, enable the server, and enter the same URL and password in PanelPC. The default port is `4455`.

Supported actions include:

- Controlling and muting an OBS input.
- Changing the current program scene.
- Toggling recording.
- Toggling streaming.
- Controlling a numeric filter setting through `SetSourceFilterSettings`.

### Alpha/opacity example

1. Add a **Color Correction** filter to the OBS source.
2. Select **OBS filter parameter** for the knob turn action.
3. Select the source and filter, then use `opacity` as the setting.
4. Use a range from `0` to `1` with current OBS versions. The legacy Color Correction filter uses `0` to `100`.

Sources and filters are queried directly from OBS and appear as suggestions in the interface.

## Integration API

PanelPC exposes a versioned HTTP API for local integrations at `http://127.0.0.1:8765/api/v1`. It listens only on loopback by default and requires the persistent bearer token stored in `~/.config/panelpc/config.json` with `0600` permissions.

Read the current device, visualizer, profile, and knob state:

```bash
PANELPC_TOKEN=$(jq -r '.api.token' ~/.config/panelpc/config.json)
curl --fail --silent \
  -H "Authorization: Bearer $PANELPC_TOKEN" \
  http://127.0.0.1:8765/api/v1/state
```

Launch the action already assigned to knob 2's click:

```bash
curl --fail --silent \
  -H "Authorization: Bearer $PANELPC_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{"knob":2,"kind":"click"}' \
  http://127.0.0.1:8765/api/v1/actions
```

Launch knob 1's configured turn action at its midpoint:

```bash
curl --fail --silent \
  -H "Authorization: Bearer $PANELPC_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{"knob":1,"kind":"turn","value":128}' \
  http://127.0.0.1:8765/api/v1/actions
```

Knob numbers in this public API are `1` through `4`; turn values are `0` through `255`. The endpoint can only trigger actions already present in the active profile. It does not accept a shell command, OBS request, or audio target in the HTTP payload. Turn coalescing, command rate limits, and the bounded action queue apply equally to physical and API-generated events.

## RGB lighting

Each knob has its own color and supports two basic modes:

- **Static:** keeps the selected color and brightness.
- **Track dial:** uses the firmware's black-to-color volume gradient so the LED follows the knob position without generating a USB write for every turn.

Global brightness limits all four colors before they are sent to the device.

## Using the PCPanel as an audio visualizer

PanelPC can repurpose all four physical RGB rings as a real-time audio visualizer. This is independent from the knob assignments: turns and clicks continue to control audio, OBS, commands, or profiles while the lighting reacts to a selected audio stream.

The visualizer can listen to:

- The default output or input.
- A specific PipeWire/PulseAudio device.
- The output stream of a specific application.

The minimum and maximum colors, brightness, dB floor and ceiling, and an update rate from 5 to 30 frames per second are configurable. Audio is captured continuously from a single PipeWire/PulseAudio stream, while RGB writes remain bounded and coalesced. These modes require `parec`, normally provided by the PulseAudio compatibility tools used with PipeWire.

### VU Toy: four-segment level visualizer

**VU Toy** transforms the complete PCPanel into a four-segment volume meter. The rings illuminate progressively from left to right as the selected signal gets louder: the first knob represents the beginning of the configured dB range and the fourth represents its highest level. Colors are interpolated between the selected minimum and maximum colors, giving the hardware the appearance of a compact physical VU display.

This mode is useful for watching overall microphone activity, application output, a capture interface, or the system mix without keeping an on-screen meter visible.

### Four-band spectrum analyzer

The **four-band spectrum analyzer** turns the four knobs into independent frequency indicators. PanelPC performs a real-time FFT on the selected stream, and each ring responds only to its assigned part of the spectrum:

- Bass: 60–250 Hz.
- Low mids: 250 Hz–1 kHz.
- High mids: 1–4 kHz.
- Treble: 4–20 kHz.

Each ring changes intensity independently, so bass hits, voices, instruments, and high-frequency detail produce visibly different patterns across the physical panel. Together, the four knobs act as a small hardware spectrum display rather than a single volume bar.

## Profiles

Each profile stores all four assignments and the complete lighting configuration, including level-meter and spectrum modes. OBS settings and its password are global. The top selector changes the active profile, while **New profile** copies the currently visible configuration. A knob click can also use the **Switch profile** action, and the change is persisted immediately.

## Safety recommendations

- **Do not run PanelPC or `install.sh` as root.** Audio, OBS, and desktop-session permissions belong to the logged-in user.
- Do not run PanelPC at the same time as another PCPanel controller. Two processes competing for the same HID device can produce unreliable behavior.
- Keep the web interface on its default loopback address. Do not expose it to a LAN or the internet.
- Shell actions intentionally execute `/bin/sh -c`. Review every configured command and never paste untrusted commands into the interface.
- Inspect `install.sh` and verify `SHA256SUMS` before installing a downloaded release.
- Do not apply unrelated driver, group, or `udev` instructions from another PCPanel application to PanelPC.

The configuration, including the OBS password and persistent integration token, is stored with `0600` permissions in `~/.config/panelpc/config.json`. The legacy web interface uses a separate random per-process token embedded into its page. The integration API requires its bearer token on every request.

## Continuous integration and release ZIPs

The GitHub Actions workflow runs the full test suite with Go's race detector and runs `go vet`. After those checks pass, it builds static Linux binaries for `amd64` and `arm64` and produces:

- `panelpc-linux-amd64.zip`
- `panelpc-linux-arm64.zip`

Each archive contains the binary, `install.sh`, the systemd user-service template, this README, and its interface screenshot. Pushes and pull requests retain the ZIPs as CI artifacts. Pushing a tag beginning with `v`, such as `v0.1.0`, creates a GitHub Release containing both ZIPs and `SHA256SUMS`.
