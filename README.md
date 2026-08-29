# PanelPC

PanelPC is a small native Linux controller for the PCPanel Lite/Mini with four clickable RGB knobs. It controls PipeWire/PulseAudio audio, device lighting, and OBS Studio through WebSocket 5 from an embedded local web interface.

## Motivation

PanelPC exists because the two available solutions we tested—the official application and a community alternative—did not work adequately on Linux. The observed problems included an impractical interface, poor PipeWire/PulseAudio channel selection, limited OBS integration, and, during one test, a system that became unresponsive when a knob was moved even after the `udev` rules had been configured.

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

## Device permissions

Install the included rule:

```bash
sudo install -m 0644 packaging/70-panelpc.rules /etc/udev/rules.d/70-panelpc.rules
sudo udevadm control --reload-rules
sudo udevadm trigger --subsystem-match=hidraw
```

The rule uses `TAG+="uaccess"`; it does not use the unsafe `MODE="0666"` setting or grant access to every HID device. Reconnect the PCPanel and confirm that the interface reports it as connected.

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

## Automatic startup

After copying the binary to `~/.local/bin/panelpc`, install the user service:

```bash
mkdir -p ~/.config/systemd/user
cp packaging/panelpc.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now panelpc.service
```

The configuration, including the OBS password, is stored with `0600` permissions in `~/.config/panelpc/config.json`. The API listens only on loopback and requires a random token embedded into the page when PanelPC starts.
