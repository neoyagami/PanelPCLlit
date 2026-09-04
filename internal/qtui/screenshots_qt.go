//go:build qt

package qtui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	qt "github.com/mappu/miqt/qt6"

	"panelpc/internal/desktopapps"
)

// SaveScreenshots renders representative application states directly through
// Qt. This keeps documentation images synchronized with the real interface.
func (w *Window) SaveScreenshots(directory string) error {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	w.Resize(1600, 1000)
	w.Show()
	qt.QCoreApplication_ProcessEvents()
	w.deviceStatus.SetText("PCPanel Mini connected")
	w.deviceStatus.SetStyleSheet("color: #62dda7; font-weight: 700; padding: 8px 12px;")

	w.prepareTurnScreenshot()
	w.repaintScreenshot()
	if err := saveWidgetPNG(w.QWidget, filepath.Join(directory, "interface.png")); err != nil {
		return err
	}

	w.prepareClickScreenshot()
	w.repaintScreenshot()
	if err := saveWidgetPNG(w.QWidget, filepath.Join(directory, "interface-actions.png")); err != nil {
		return err
	}

	w.openSettings()
	w.settings.Repaint()
	qt.QCoreApplication_ProcessEvents()
	if err := saveWidgetPNG(w.settings.QWidget, filepath.Join(directory, "interface-settings.png")); err != nil {
		return err
	}
	w.settings.Hide()
	return nil
}

func (w *Window) prepareTurnScreenshot() {
	w.lightingOpen = true
	w.lightingBody.SetVisible(true)
	w.lightingToggle.SetText("Hide")
	setOptionValue(w.mode, lightingOptions, "spectrum")
	setOptionValue(w.vuSource, sourceOptions, "app")
	w.vuTarget.SetCurrentText("Firefox")
	w.globalBright.SetValue(78)
	w.vuBrightness.SetValue(85)
	w.vuFPS.SetValue(20)
	w.updateLightingVisibility()

	labels := []string{"Music", "Microphone", "Camera alpha", "Automation"}
	for index := range w.knobs {
		w.knobs[index].label.SetText(labels[index])
		w.knobs[index].tabs.SetCurrentIndex(0)
	}
	setOptionValue(w.knobs[0].turnKind, turnOptions, "app")
	w.knobs[0].turnTarget.SetCurrentText("Spotify")
	w.knobs[0].minPercent.SetValue(0)
	w.knobs[0].maxPercent.SetValue(100)
	setOptionValue(w.knobs[1].turnKind, turnOptions, "input")
	w.knobs[1].minPercent.SetValue(0)
	w.knobs[1].maxPercent.SetValue(115)
	setOptionValue(w.knobs[2].turnKind, turnOptions, "obs_filter")
	w.knobs[2].filterSource.SetCurrentText("Camera")
	w.knobs[2].filterName.SetCurrentText("Color Correction")
	w.knobs[2].filterSetting.SetText("opacity")
	w.knobs[2].filterMin.SetValue(0)
	w.knobs[2].filterMax.SetValue(1)
	setOptionValue(w.knobs[3].turnKind, turnOptions, "shell")
	w.knobs[3].turnCommand.SetPlainText(`/bin/echo "$LEVEL" > /tmp/panel-level`)
	w.knobs[3].turnRate.SetValue(250)
	for index := range w.knobs {
		w.updateKnobVisibility(index)
	}
}

func (w *Window) prepareClickScreenshot() {
	w.lightingOpen = false
	w.lightingBody.SetVisible(false)
	w.lightingToggle.SetText("Show")
	for index := range w.knobs {
		w.knobs[index].tabs.SetCurrentIndex(1)
	}

	setOptionValue(w.knobs[0].pressKind, pressOptions, "application")
	applications, _ := desktopapps.Discover()
	w.setDesktopApplications(applications)
	replaceNamedChoices(w.knobs[0].pressTarget, w.desktopApps)
	selected := 0
	for index, item := range w.desktopApps {
		if strings.Contains(strings.ToLower(item.label), "obs") {
			selected = index
			break
		}
	}
	if len(w.desktopApps) != 0 {
		w.knobs[0].pressTarget.SetCurrentIndex(selected)
	}
	setOptionValue(w.knobs[1].pressKind, pressOptions, "mute_turn")
	setOptionValue(w.knobs[2].pressKind, pressOptions, "obs_scene")
	w.knobs[2].pressTarget.SetCurrentText("Starting Soon")
	setOptionValue(w.knobs[3].pressKind, pressOptions, "shell")
	w.knobs[3].pressCommand.SetPlainText(`/usr/bin/notify-send "PanelPC" "Stream ready"`)
	for index := range w.knobs {
		w.updateKnobVisibility(index)
	}
}

func (w *Window) repaintScreenshot() {
	w.Hide()
	qt.QCoreApplication_ProcessEvents()
	w.Show()
	if w.scroll != nil {
		w.scroll.VerticalScrollBar().SetValue(0)
	}
	w.UpdateGeometry()
	w.Repaint()
	qt.QCoreApplication_ProcessEvents()
}

func saveWidgetPNG(widget *qt.QWidget, path string) error {
	pixmap := widget.Grab()
	if pixmap.IsNull() || !pixmap.Save2(path, "PNG") {
		return fmt.Errorf("could not save screenshot %s", path)
	}
	return nil
}
