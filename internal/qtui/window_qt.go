//go:build qt

package qtui

import (
	"fmt"
	"sort"
	"strings"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"

	"panelpc/internal/audio"
	"panelpc/internal/config"
	"panelpc/internal/desktopapps"
	"panelpc/internal/device"
	"panelpc/internal/engine"
	"panelpc/internal/server"
)

type option struct {
	label string
	value string
}

var turnOptions = []option{
	{"No action", "none"},
	{"Output volume", "output"},
	{"Microphone volume", "input"},
	{"Output device", "output_device"},
	{"Input device", "input_device"},
	{"Application volume", "app"},
	{"OBS input volume", "obs_input"},
	{"OBS filter parameter", "obs_filter"},
	{"Run shell command", "shell"},
}

var pressOptions = []option{
	{"No action", "none"},
	{"Mute turn channel", "mute_turn"},
	{"Switch OBS scene", "obs_scene"},
	{"Toggle OBS recording", "obs_toggle_record"},
	{"Toggle OBS streaming", "obs_toggle_stream"},
	{"Mute OBS input", "obs_toggle_input_mute"},
	{"Switch profile", "profile"},
	{"Launch desktop application", "application"},
	{"Run shell command", "shell"},
}

var lightingOptions = []option{
	{"Independent RGB", "dials"},
	{"Level visualizer · 4 segments", "vu"},
	{"Spectrum visualizer · 4 bands", "spectrum"},
}

var sourceOptions = []option{
	{"Default output", "output"},
	{"Default input", "input"},
	{"Application output", "app"},
	{"Output device", "output_device"},
	{"Input device", "input_device"},
}

type knobWidgets struct {
	dial         *qt.QDial
	dialDragging bool
	value        *qt.QLabel
	tabs         *qt.QTabWidget
	label        *qt.QLineEdit
	color        *qt.QPushButton
	colorValue   string
	track        *qt.QCheckBox

	turnKind       *qt.QComboBox
	turnTarget     *qt.QComboBox
	turnTargetWrap *qt.QWidget
	percentWrap    *qt.QWidget
	minPercent     *qt.QDoubleSpinBox
	maxPercent     *qt.QDoubleSpinBox
	filterWrap     *qt.QWidget
	filterSource   *qt.QComboBox
	filterName     *qt.QComboBox
	filterSetting  *qt.QLineEdit
	filterMin      *qt.QDoubleSpinBox
	filterMax      *qt.QDoubleSpinBox
	shellTurnWrap  *qt.QWidget
	turnCommand    *qt.QPlainTextEdit
	turnRate       *qt.QSpinBox

	pressKind       *qt.QComboBox
	pressTarget     *qt.QComboBox
	pressTargetWrap *qt.QWidget
	shellPressWrap  *qt.QWidget
	pressCommand    *qt.QPlainTextEdit
}

type Window struct {
	*qt.QMainWindow
	controller *server.Server
	device     *device.Manager
	engine     *engine.Engine
	audio      *audio.Controller
	listen     string
	preview    bool
	loading    bool
	quitting   bool
	cfg        config.Config

	deviceStatus   *qt.QLabel
	notice         *qt.QLabel
	profile        *qt.QComboBox
	lightingBody   *qt.QWidget
	lightingToggle *qt.QPushButton
	lightingOpen   bool
	mode           *qt.QComboBox
	globalBright   *qt.QSpinBox
	reactiveWrap   *qt.QWidget
	vuSource       *qt.QComboBox
	vuTarget       *qt.QComboBox
	vuMinColor     *qt.QPushButton
	vuMinValue     string
	vuMaxColor     *qt.QPushButton
	vuMaxValue     string
	vuBrightness   *qt.QSpinBox
	vuMinDB        *qt.QDoubleSpinBox
	vuMaxDB        *qt.QDoubleSpinBox
	vuFPS          *qt.QSpinBox
	visualHelp     *qt.QLabel

	obsURL      *qt.QLineEdit
	obsPassword *qt.QLineEdit
	apiToken    *qt.QLineEdit
	settings    *qt.QDialog
	tray        *qt.QSystemTrayIcon
	trayMenu    *qt.QMenu
	trayReady   bool
	scroll      *qt.QScrollArea
	diagnostics *qt.QLabel
	levels      [4]*qt.QProgressBar
	knobs       [4]knobWidgets
	timer       *qt.QTimer

	apps          []string
	outputDevices []choice
	inputDevices  []choice
	obsInputs     []string
	desktopApps   []choice
}

type choice struct {
	label string
	value string
}

func NewWindow(controller *server.Server, dev *device.Manager, eng *engine.Engine, aud *audio.Controller, listen string, preview bool) *Window {
	w := &Window{
		QMainWindow:  qt.NewQMainWindow2(),
		controller:   controller,
		device:       dev,
		engine:       eng,
		audio:        aud,
		listen:       listen,
		preview:      preview,
		lightingOpen: true,
	}
	w.SetWindowTitle("PanelPC · PCPanel for Linux")
	w.Resize(1500, 900)
	w.SetMinimumWidth(1050)
	w.build()
	w.load(controller.Config())
	w.startStatusTimer()
	w.refreshTargets()
	return w
}

func (w *Window) build() {
	root := qt.NewQWidget2()
	rootLayout := qt.NewQVBoxLayout2()
	rootLayout.SetContentsMargins(18, 16, 18, 16)
	rootLayout.SetSpacing(12)
	root.SetLayout(rootLayout.QLayout)

	header := qt.NewQHBoxLayout2()
	titles := qt.NewQWidget2()
	titleLayout := qt.NewQVBoxLayout2()
	titleLayout.SetContentsMargins(0, 0, 0, 0)
	title := qt.NewQLabel3("PanelPC")
	title.SetStyleSheet("font-size: 24px; font-weight: 700; color: #f7f9fc;")
	subtitle := qt.NewQLabel3("Native PCPanel control for Linux · direct USB, PipeWire/PulseAudio and OBS")
	subtitle.SetStyleSheet("color: #8998ae;")
	titleLayout.AddWidget(title.QWidget)
	titleLayout.AddWidget(subtitle.QWidget)
	titles.SetLayout(titleLayout.QLayout)
	header.AddWidget2(titles, 1)
	w.deviceStatus = qt.NewQLabel3("Looking for PCPanel…")
	w.deviceStatus.SetStyleSheet("color: #f2b66d; font-weight: 700; padding: 8px 12px;")
	header.AddWidget(w.deviceStatus.QWidget)
	rootLayout.AddLayout(header.QLayout)

	toolbar := qt.NewQWidget2()
	toolbarLayout := qt.NewQHBoxLayout2()
	toolbarLayout.SetContentsMargins(0, 0, 0, 0)
	toolbarLayout.AddWidget(qt.NewQLabel3("Active profile").QWidget)
	w.profile = qt.NewQComboBox2()
	w.profile.SetMinimumWidth(180)
	toolbarLayout.AddWidget(w.profile.QWidget)
	newProfile := qt.NewQPushButton3("New profile")
	toolbarLayout.AddWidget(newProfile.QWidget)
	settingsButton := qt.NewQPushButton3("Settings…")
	toolbarLayout.AddWidget(settingsButton.QWidget)
	toolbarLayout.AddStretch()
	w.notice = qt.NewQLabel3("")
	w.notice.SetStyleSheet("color: #62dda7;")
	toolbarLayout.AddWidget(w.notice.QWidget)
	save := qt.NewQPushButton3("Save changes")
	save.SetStyleSheet("background: #2874e8; border-color: #4d92fa;")
	toolbarLayout.AddWidget(save.QWidget)
	toolbar.SetLayout(toolbarLayout.QLayout)
	rootLayout.AddWidget(toolbar)

	w.buildLighting(rootLayout)

	knobRow := qt.NewQHBoxLayout2()
	knobRow.SetSpacing(10)
	for index := range w.knobs {
		card := w.buildKnob(index)
		knobRow.AddWidget2(card, 1)
	}
	rootLayout.AddLayout(knobRow.QLayout)

	w.buildDiagnostics(rootLayout)

	scroll := qt.NewQScrollArea2()
	w.scroll = scroll
	scroll.SetWidgetResizable(true)
	scroll.SetWidget(root)
	w.SetCentralWidget(scroll.QWidget)

	save.OnClicked(func() { w.save(true) })
	newProfile.OnClicked(w.createProfile)
	settingsButton.OnClicked(w.openSettings)
	w.profile.OnCurrentTextChanged(w.profileChanged)
	w.buildSettings()
	w.buildTray()
}

func (w *Window) buildLighting(root *qt.QVBoxLayout) {
	box := qt.NewQGroupBox3("Device lighting and audio visualizer")
	outer := qt.NewQVBoxLayout2()
	outer.SetSpacing(8)
	top := qt.NewQHBoxLayout2()
	top.AddWidget(qt.NewQLabel3("Use independent dial colors, a four-segment level meter, or a four-band spectrum display.").QWidget)
	top.AddStretch()
	w.lightingToggle = qt.NewQPushButton3("Hide")
	top.AddWidget(w.lightingToggle.QWidget)
	outer.AddLayout(top.QLayout)

	w.lightingBody = qt.NewQWidget2()
	grid := qt.NewQGridLayout2()
	grid.SetContentsMargins(0, 4, 0, 0)
	grid.SetSpacing(8)
	w.mode = newOptionsCombo(lightingOptions)
	w.globalBright = newSpin(0, 100, "%")
	grid.AddWidget2(labeled("Mode", w.mode.QWidget), 0, 0)
	grid.AddWidget2(labeled("Dial brightness", w.globalBright.QWidget), 0, 1)

	w.reactiveWrap = qt.NewQWidget2()
	reactive := qt.NewQGridLayout2()
	reactive.SetContentsMargins(0, 0, 0, 0)
	reactive.SetSpacing(8)
	w.vuSource = newOptionsCombo(sourceOptions)
	w.vuTarget = qt.NewQComboBox2()
	w.vuTarget.SetEditable(true)
	w.vuMinColor = qt.NewQPushButton3("")
	w.vuMaxColor = qt.NewQPushButton3("")
	w.vuBrightness = newSpin(0, 100, "%")
	w.vuMinDB = newDouble(-100, 0, 1)
	w.vuMaxDB = newDouble(-40, 12, 1)
	w.vuFPS = newSpin(5, 30, " Hz")
	reactive.AddWidget2(labeled("Audio source", w.vuSource.QWidget), 0, 0)
	reactive.AddWidget2(labeled("Application or device", w.vuTarget.QWidget), 0, 1)
	reactive.AddWidget2(labeled("Minimum color", w.vuMinColor.QWidget), 0, 2)
	reactive.AddWidget2(labeled("Maximum color", w.vuMaxColor.QWidget), 0, 3)
	reactive.AddWidget2(labeled("Brightness", w.vuBrightness.QWidget), 1, 0)
	reactive.AddWidget2(labeled("Floor", w.vuMinDB.QWidget), 1, 1)
	reactive.AddWidget2(labeled("Ceiling", w.vuMaxDB.QWidget), 1, 2)
	reactive.AddWidget2(labeled("Updates", w.vuFPS.QWidget), 1, 3)
	w.visualHelp = qt.NewQLabel3("")
	w.visualHelp.SetWordWrap(true)
	w.visualHelp.SetStyleSheet("color: #8998ae;")
	reactive.AddWidget3(w.visualHelp.QWidget, 2, 0, 1, 4)
	w.reactiveWrap.SetLayout(reactive.QLayout)
	grid.AddWidget3(w.reactiveWrap, 1, 0, 1, 4)
	w.lightingBody.SetLayout(grid.QLayout)
	outer.AddWidget(w.lightingBody)
	box.SetLayout(outer.QLayout)
	root.AddWidget(box.QWidget)

	w.lightingToggle.OnClicked(func() {
		w.lightingOpen = !w.lightingOpen
		w.lightingBody.SetVisible(w.lightingOpen)
		if w.lightingOpen {
			w.lightingToggle.SetText("Hide")
		} else {
			w.lightingToggle.SetText("Show")
		}
	})
	w.mode.OnCurrentTextChanged(func(string) { w.updateLightingVisibility() })
	w.vuSource.OnCurrentTextChanged(func(string) { w.populateVUTarget() })
	w.vuMinColor.OnClicked(func() { w.chooseColor(w.vuMinColor, &w.vuMinValue) })
	w.vuMaxColor.OnClicked(func() { w.chooseColor(w.vuMaxColor, &w.vuMaxValue) })
}

func (w *Window) buildKnob(index int) *qt.QWidget {
	k := &w.knobs[index]
	box := qt.NewQGroupBox3(fmt.Sprintf("Knob %d", index+1))
	box.SetMinimumWidth(275)
	layout := qt.NewQVBoxLayout2()
	layout.SetSpacing(8)

	dialRow := qt.NewQHBoxLayout2()
	k.dial = qt.NewQDial2()
	k.dial.SetRange(0, 255)
	k.dial.SetNotchesVisible(true)
	k.dial.SetFixedSize2(88, 88)
	k.dial.SetToolTip("Current physical position. Drag and release to test the turn action.")
	dialRow.AddStretch()
	dialRow.AddWidget(k.dial.QWidget)
	k.value = qt.NewQLabel3("0 · 0%")
	k.value.SetMinimumWidth(58)
	k.value.SetStyleSheet("font-weight: 700; color: #9fbfff;")
	dialRow.AddWidget(k.value.QWidget)
	dialRow.AddStretch()
	layout.AddLayout(dialRow.QLayout)

	k.label = qt.NewQLineEdit2()
	layout.AddWidget(labeled("Name", k.label.QWidget))
	k.color = qt.NewQPushButton3("")
	k.track = qt.NewQCheckBox3("LED brightness follows dial position")
	colorRow := qt.NewQHBoxLayout2()
	colorRow.AddWidget2(labeled("Ring color", k.color.QWidget), 1)
	colorRow.AddWidget2(k.track.QWidget, 2)
	layout.AddLayout(colorRow.QLayout)

	tabs := qt.NewQTabWidget2()
	k.tabs = tabs
	turnTab := qt.NewQWidget2()
	turnLayout := qt.NewQVBoxLayout2()
	turnLayout.SetContentsMargins(8, 8, 8, 8)
	turnLayout.SetSpacing(7)
	k.turnKind = newOptionsCombo(turnOptions)
	turnLayout.AddWidget(labeled("On turn", k.turnKind.QWidget))
	k.turnTarget = qt.NewQComboBox2()
	k.turnTarget.SetEditable(true)
	k.turnTargetWrap = labeled("Target", k.turnTarget.QWidget)
	turnLayout.AddWidget(k.turnTargetWrap)
	k.minPercent = newDouble(0, 150, 1)
	k.minPercent.SetSuffix(" %")
	k.maxPercent = newDouble(0, 150, 1)
	k.maxPercent.SetSuffix(" %")
	rangeRow := qt.NewQHBoxLayout2()
	rangeRow.AddWidget2(labeled("Minimum", k.minPercent.QWidget), 1)
	rangeRow.AddWidget2(labeled("Maximum", k.maxPercent.QWidget), 1)
	k.percentWrap = qt.NewQWidget2()
	k.percentWrap.SetLayout(rangeRow.QLayout)
	turnLayout.AddWidget(k.percentWrap)

	k.filterSource = editableCombo("Camera source")
	k.filterName = editableCombo("Color Correction")
	k.filterSetting = qt.NewQLineEdit3("opacity")
	k.filterMin = newDouble(-100000, 100000, 3)
	k.filterMax = newDouble(-100000, 100000, 3)
	filterLayout := qt.NewQVBoxLayout2()
	filterLayout.SetContentsMargins(0, 0, 0, 0)
	filterLayout.SetSpacing(6)
	filterLayout.AddWidget(labeled("OBS source", k.filterSource.QWidget))
	filterLayout.AddWidget(labeled("Filter", k.filterName.QWidget))
	filterLayout.AddWidget(labeled("Property", k.filterSetting.QWidget))
	filterRange := qt.NewQHBoxLayout2()
	filterRange.AddWidget2(labeled("Minimum", k.filterMin.QWidget), 1)
	filterRange.AddWidget2(labeled("Maximum", k.filterMax.QWidget), 1)
	filterLayout.AddLayout(filterRange.QLayout)
	k.filterWrap = qt.NewQWidget2()
	k.filterWrap.SetLayout(filterLayout.QLayout)
	turnLayout.AddWidget(k.filterWrap)

	k.turnCommand = qt.NewQPlainTextEdit2()
	k.turnCommand.SetPlaceholderText("/bin/echo $LEVEL > /tmp/level")
	k.turnCommand.SetFixedHeight(66)
	k.turnRate = newSpin(50, 60000, " ms")
	shellTurnLayout := qt.NewQVBoxLayout2()
	shellTurnLayout.SetContentsMargins(0, 0, 0, 0)
	shellTurnLayout.AddWidget(labeled("Command via /bin/sh -c", k.turnCommand.QWidget))
	shellTurnLayout.AddWidget(labeled("Minimum interval", k.turnRate.QWidget))
	k.shellTurnWrap = qt.NewQWidget2()
	k.shellTurnWrap.SetLayout(shellTurnLayout.QLayout)
	turnLayout.AddWidget(k.shellTurnWrap)
	turnLayout.AddStretch()
	turnTab.SetLayout(turnLayout.QLayout)
	tabs.AddTab(turnTab, "Turn")

	pressTab := qt.NewQWidget2()
	pressLayout := qt.NewQVBoxLayout2()
	pressLayout.SetContentsMargins(8, 8, 8, 8)
	pressLayout.SetSpacing(7)
	k.pressKind = newOptionsCombo(pressOptions)
	pressLayout.AddWidget(labeled("On click", k.pressKind.QWidget))
	k.pressTarget = qt.NewQComboBox2()
	k.pressTarget.SetEditable(true)
	k.pressTargetWrap = labeled("Target", k.pressTarget.QWidget)
	pressLayout.AddWidget(k.pressTargetWrap)
	k.pressCommand = qt.NewQPlainTextEdit2()
	k.pressCommand.SetPlaceholderText("/bin/echo click > /tmp/click")
	k.pressCommand.SetFixedHeight(82)
	shellPressLayout := qt.NewQVBoxLayout2()
	shellPressLayout.SetContentsMargins(0, 0, 0, 0)
	shellPressLayout.AddWidget(labeled("Command via /bin/sh -c", k.pressCommand.QWidget))
	k.shellPressWrap = qt.NewQWidget2()
	k.shellPressWrap.SetLayout(shellPressLayout.QLayout)
	pressLayout.AddWidget(k.shellPressWrap)
	pressLayout.AddStretch()
	pressTab.SetLayout(pressLayout.QLayout)
	tabs.AddTab(pressTab, "Click")
	layout.AddWidget(tabs.QWidget)

	testClick := qt.NewQPushButton3("Test click")
	layout.AddWidget(testClick.QWidget)
	box.SetLayout(layout.QLayout)

	k.color.OnClicked(func() { w.chooseColor(k.color, &k.colorValue) })
	k.turnKind.OnCurrentTextChanged(func(string) {
		w.updateKnobVisibility(index)
		w.populateTurnTarget(index)
	})
	k.pressKind.OnCurrentTextChanged(func(string) {
		w.updateKnobVisibility(index)
		w.populatePressTarget(index)
	})
	k.filterSource.OnCurrentTextChanged(func(string) { w.refreshOBSFilters(index) })
	k.dial.OnSliderPressed(func() { k.dialDragging = true })
	k.dial.OnSliderReleased(func() {
		k.dialDragging = false
		if w.save(false) {
			w.engine.Inject(device.Event{Kind: "turn", Knob: index, Value: k.dial.Value()})
		}
	})
	testClick.OnClicked(func() {
		if w.save(false) {
			w.engine.Inject(device.Event{Kind: "press", Knob: index, Value: 1})
		}
	})
	return box.QWidget
}

func (w *Window) buildSettings() {
	w.settings = qt.NewQDialog(w.QWidget)
	w.settings.SetWindowTitle("PanelPC settings")
	w.settings.SetModal(false)
	w.settings.Resize(820, 260)
	root := qt.NewQVBoxLayout2()
	root.SetContentsMargins(12, 12, 12, 12)
	root.SetSpacing(8)

	row := qt.NewQHBoxLayout2()
	row.SetSpacing(10)
	obs := qt.NewQGroupBox3("OBS WebSocket 5")
	obsLayout := qt.NewQFormLayout2()
	obsLayout.SetContentsMargins(10, 10, 10, 10)
	obsLayout.SetHorizontalSpacing(10)
	obsLayout.SetVerticalSpacing(7)
	obsLayout.SetFieldGrowthPolicy(qt.QFormLayout__AllNonFixedFieldsGrow)
	w.obsURL = qt.NewQLineEdit2()
	w.obsURL.SetPlaceholderText("ws://127.0.0.1:4455")
	w.obsPassword = qt.NewQLineEdit2()
	w.obsPassword.SetEchoMode(qt.QLineEdit__Password)
	obsLayout.AddRow3("URL", w.obsURL.QWidget)
	obsLayout.AddRow3("Password", w.obsPassword.QWidget)
	test := qt.NewQPushButton3("Save and test OBS")
	obsLayout.AddRow3("", test.QWidget)
	obs.SetLayout(obsLayout.QLayout)
	row.AddWidget2(obs.QWidget, 1)

	api := qt.NewQGroupBox3("Local integration API")
	apiLayout := qt.NewQFormLayout2()
	apiLayout.SetContentsMargins(10, 10, 10, 10)
	apiLayout.SetHorizontalSpacing(10)
	apiLayout.SetVerticalSpacing(7)
	apiLayout.SetFieldGrowthPolicy(qt.QFormLayout__AllNonFixedFieldsGrow)
	apiHelp := qt.NewQLabel3("Read knob state and trigger configured actions from other local tools.")
	apiHelp.SetStyleSheet("color: #8998ae;")
	apiLayout.AddRowWithWidget(apiHelp.QWidget)
	address := qt.NewQLineEdit3("http://" + w.listen + "/api/v1")
	address.SetReadOnly(true)
	w.apiToken = qt.NewQLineEdit2()
	w.apiToken.SetReadOnly(true)
	w.apiToken.SetEchoMode(qt.QLineEdit__PasswordEchoOnEdit)
	apiLayout.AddRow3("Address", address.QWidget)
	apiLayout.AddRow3("Bearer token", w.apiToken.QWidget)
	api.SetLayout(apiLayout.QLayout)
	row.AddWidget2(api.QWidget, 1)
	root.AddLayout(row.QLayout)
	buttons := qt.NewQDialogButtonBox4(qt.QDialogButtonBox__Save | qt.QDialogButtonBox__Cancel)
	root.AddWidget(buttons.QWidget)
	w.settings.SetLayout(root.QLayout)

	test.OnClicked(func() {
		if !w.saveSettings() {
			return
		}
		test.SetEnabled(false)
		w.setNotice("Testing OBS…", false)
		go func() {
			err := w.engine.TestOBS()
			mainthread.Start(func() {
				test.SetEnabled(true)
				if err != nil {
					w.setNotice("OBS: "+err.Error(), true)
					return
				}
				w.setNotice("OBS responded successfully.", false)
				w.refreshOBSInputs()
			})
		}()
	})
	buttons.OnAccepted(func() {
		if w.saveSettings() {
			w.settings.Accept()
		}
	})
	buttons.OnRejected(w.settings.Reject)
}

func (w *Window) buildTray() {
	w.trayReady = qt.QSystemTrayIcon_IsSystemTrayAvailable()
	icon := panelIcon()
	qt.QGuiApplication_SetWindowIcon(icon)
	w.SetWindowIcon(icon)
	if !w.trayReady {
		return
	}
	w.tray = qt.NewQSystemTrayIcon2(icon)
	w.tray.SetToolTip("PanelPC · PCPanel controller")
	w.trayMenu = qt.NewQMenu2()
	open := w.trayMenu.AddActionWithText("Open PanelPC")
	settings := w.trayMenu.AddActionWithText("Settings…")
	about := w.trayMenu.AddActionWithText("About PanelPC")
	w.trayMenu.AddSeparator()
	quit := w.trayMenu.AddActionWithText("Quit")
	w.trayMenu.SetDefaultAction(open)
	w.tray.SetContextMenu(w.trayMenu)
	open.OnTriggered(w.showMainWindow)
	settings.OnTriggered(w.openSettings)
	about.OnTriggered(w.showAbout)
	quit.OnTriggered(func() {
		w.quitting = true
		w.tray.Hide()
		qt.QCoreApplication_Quit()
	})
	w.tray.OnActivated(func(reason qt.QSystemTrayIcon__ActivationReason) {
		if reason == qt.QSystemTrayIcon__Trigger || reason == qt.QSystemTrayIcon__DoubleClick {
			w.showMainWindow()
		}
	})
	w.tray.Show()
	qt.QGuiApplication_SetQuitOnLastWindowClosed(false)
	w.OnCloseEvent(func(super func(event *qt.QCloseEvent), event *qt.QCloseEvent) {
		if w.trayReady && !w.quitting {
			event.Ignore()
			w.Hide()
			return
		}
		super(event)
	})
}

func (w *Window) showMainWindow() {
	w.ShowNormal()
	w.Raise()
	w.ActivateWindow()
	if current := w.controller.Config(); current.ActiveProfile != w.cfg.ActiveProfile {
		w.load(current)
	}
}

// TrayAvailable reports whether closing or background startup can rely on a
// desktop tray. The executable falls back to a visible window when it cannot.
func (w *Window) TrayAvailable() bool { return w.trayReady }

func (w *Window) openSettings() {
	cfg := w.controller.Config()
	w.obsURL.SetText(cfg.OBS.URL)
	w.obsPassword.SetText(cfg.OBS.Password)
	w.apiToken.SetText(cfg.API.Token)
	w.settings.Show()
	w.settings.Raise()
	w.settings.ActivateWindow()
}

func (w *Window) showAbout() {
	dialog := qt.NewQDialog(w.QWidget)
	defer dialog.Delete()
	dialog.SetWindowTitle("About PanelPC")
	dialog.SetWindowIcon(w.WindowIcon())
	dialog.SetMinimumWidth(430)
	layout := qt.NewQVBoxLayout(dialog.QWidget)
	layout.SetContentsMargins(28, 24, 28, 20)
	layout.SetSpacing(12)

	title := qt.NewQLabel3("PanelPC")
	title.SetAlignment(qt.AlignCenter)
	title.SetStyleSheet("font-size: 24px; font-weight: 700;")
	layout.AddWidget(title.QWidget)

	details := qt.NewQLabel3(`<div style="text-align:center">
Native PCPanel Lite/Mini controller for Linux<br><br>
Development build · neoyagami · 2026<br>
Built with AI tools<br>
Licensed under GNU GPLv3 or later<br><br>
<a href="https://github.com/neoyagami/PanelPCLlit">Download and source code</a>
</div>`)
	details.SetAlignment(qt.AlignCenter)
	details.SetOpenExternalLinks(true)
	details.SetTextInteractionFlags(qt.TextBrowserInteraction)
	layout.AddWidget(details.QWidget)

	buttons := qt.NewQDialogButtonBox4(qt.QDialogButtonBox__Close)
	buttons.Button(qt.QDialogButtonBox__Close).SetText("Close")
	buttons.OnRejected(dialog.Reject)
	layout.AddWidget(buttons.QWidget)
	dialog.Exec()
}

func (w *Window) saveSettings() bool {
	// Integration settings are global. Start from the persisted controller
	// state so saving this dialog cannot implicitly save unsaved profile edits.
	cfg := w.controller.Config()
	cfg.OBS.URL = strings.TrimSpace(w.obsURL.Text())
	cfg.OBS.Password = w.obsPassword.Text()
	cfg.Normalize()
	if err := w.controller.UpdateConfig(cfg); err != nil {
		w.setNotice("Could not save settings: "+err.Error(), true)
		return false
	}
	w.cfg = cfg.Clone()
	w.setNotice("Settings saved.", false)
	return true
}

func (w *Window) buildDiagnostics(root *qt.QVBoxLayout) {
	box := qt.NewQGroupBox3("Live diagnostics")
	layout := qt.NewQVBoxLayout2()
	w.diagnostics = qt.NewQLabel3("No events yet")
	w.diagnostics.SetWordWrap(true)
	layout.AddWidget(w.diagnostics.QWidget)
	levels := qt.NewQHBoxLayout2()
	for index := range w.levels {
		bar := qt.NewQProgressBar2()
		bar.SetRange(0, 100)
		bar.SetValue(0)
		bar.SetFormat(fmt.Sprintf("%d · %%p%%", index+1))
		w.levels[index] = bar
		levels.AddWidget2(bar.QWidget, 1)
	}
	layout.AddLayout(levels.QLayout)
	box.SetLayout(layout.QLayout)
	root.AddWidget(box.QWidget)
}

func (w *Window) load(cfg config.Config) {
	w.loading = true
	defer func() { w.loading = false }()
	w.cfg = cfg.Clone()
	w.profile.Clear()
	w.profile.AddItems(cfg.ProfileNames())
	w.profile.SetCurrentText(cfg.ActiveProfile)
	setOptionValue(w.mode, lightingOptions, cfg.Lighting.Mode)
	w.globalBright.SetValue(cfg.Lighting.GlobalBrightness)
	setOptionValue(w.vuSource, sourceOptions, cfg.Lighting.VU.SourceKind)
	w.vuTarget.SetCurrentText(cfg.Lighting.VU.Target)
	w.vuMinValue = cfg.Lighting.VU.MinColor
	w.vuMaxValue = cfg.Lighting.VU.MaxColor
	paintColorButton(w.vuMinColor, w.vuMinValue)
	paintColorButton(w.vuMaxColor, w.vuMaxValue)
	w.vuBrightness.SetValue(cfg.Lighting.VU.Brightness)
	w.vuMinDB.SetValue(cfg.Lighting.VU.MinDB)
	w.vuMaxDB.SetValue(cfg.Lighting.VU.MaxDB)
	w.vuFPS.SetValue(cfg.Lighting.VU.FPS)
	w.obsURL.SetText(cfg.OBS.URL)
	w.obsPassword.SetText(cfg.OBS.Password)
	w.apiToken.SetText(cfg.API.Token)
	for index, knob := range cfg.Knobs {
		k := &w.knobs[index]
		k.label.SetText(knob.Label)
		k.colorValue = knob.Light.Color
		paintColorButton(k.color, k.colorValue)
		k.track.SetChecked(knob.Light.TrackValue)
		setOptionValue(k.turnKind, turnOptions, knob.Turn.Kind)
		k.turnTarget.SetCurrentText(knob.Turn.Target)
		k.minPercent.SetValue(knob.Turn.MinPercent)
		k.maxPercent.SetValue(knob.Turn.MaxPercent)
		k.filterSource.SetCurrentText(knob.Turn.Source)
		k.filterName.SetCurrentText(knob.Turn.Filter)
		k.filterSetting.SetText(knob.Turn.Setting)
		k.filterMin.SetValue(knob.Turn.MinValue)
		k.filterMax.SetValue(knob.Turn.MaxValue)
		k.turnCommand.SetPlainText(knob.Turn.Command)
		k.turnRate.SetValue(knob.Turn.RateMS)
		setOptionValue(k.pressKind, pressOptions, knob.Press.Kind)
		k.pressTarget.SetCurrentText(knob.Press.Target)
		k.pressCommand.SetPlainText(knob.Press.Command)
		w.updateKnobVisibility(index)
	}
	w.updateLightingVisibility()
	w.populateAllTargets()
}

func (w *Window) readConfig() config.Config {
	cfg := w.cfg.Clone()
	cfg.ActiveProfile = strings.TrimSpace(w.profile.CurrentText())
	cfg.Lighting.Mode = optionValue(w.mode, lightingOptions)
	cfg.Lighting.GlobalBrightness = w.globalBright.Value()
	cfg.Lighting.VU = config.VU{
		SourceKind: optionValue(w.vuSource, sourceOptions),
		Target:     comboValue(w.vuTarget),
		MinColor:   w.vuMinValue,
		MaxColor:   w.vuMaxValue,
		Brightness: w.vuBrightness.Value(),
		MinDB:      w.vuMinDB.Value(),
		MaxDB:      w.vuMaxDB.Value(),
		FPS:        w.vuFPS.Value(),
	}
	for index := range cfg.Knobs {
		k := &w.knobs[index]
		cfg.Knobs[index] = config.Knob{
			Label: strings.TrimSpace(k.label.Text()),
			Light: config.KnobLighting{Color: k.colorValue, TrackValue: k.track.IsChecked()},
			Turn: config.TurnAction{
				Kind:       optionValue(k.turnKind, turnOptions),
				Target:     comboValue(k.turnTarget),
				MinPercent: k.minPercent.Value(),
				MaxPercent: k.maxPercent.Value(),
				Source:     strings.TrimSpace(k.filterSource.CurrentText()),
				Filter:     strings.TrimSpace(k.filterName.CurrentText()),
				Setting:    strings.TrimSpace(k.filterSetting.Text()),
				MinValue:   k.filterMin.Value(),
				MaxValue:   k.filterMax.Value(),
				Command:    k.turnCommand.ToPlainText(),
				RateMS:     k.turnRate.Value(),
			},
			Press: config.PressAction{
				Kind:    optionValue(k.pressKind, pressOptions),
				Target:  comboValue(k.pressTarget),
				Command: k.pressCommand.ToPlainText(),
			},
		}
	}
	return cfg
}

func (w *Window) save(collapseLighting bool) bool {
	cfg := w.readConfig()
	cfg.Normalize()
	if err := w.controller.UpdateConfig(cfg); err != nil {
		w.setNotice("Could not save: "+err.Error(), true)
		return false
	}
	w.cfg = cfg.Clone()
	if collapseLighting && w.lightingOpen {
		w.lightingOpen = false
		w.lightingBody.SetVisible(false)
		w.lightingToggle.SetText("Show")
	}
	w.setNotice("Configuration saved.", false)
	return true
}

func (w *Window) profileChanged(name string) {
	if w.loading || name == "" || name == w.cfg.ActiveProfile {
		return
	}
	// Save edits into the profile being left. The combo already displays the
	// destination, so temporarily restore the old selection while reading UI.
	w.loading = true
	w.profile.SetCurrentText(w.cfg.ActiveProfile)
	w.loading = false
	if !w.save(false) {
		return
	}
	if err := w.controller.ActivateProfile(name); err != nil {
		w.setNotice("Could not activate profile: "+err.Error(), true)
		return
	}
	w.load(w.controller.Config())
	w.setNotice(fmt.Sprintf("Profile “%s” activated.", name), false)
}

func (w *Window) createProfile() {
	var accepted bool
	name := strings.TrimSpace(qt.QInputDialog_GetText4(w.QWidget, "New profile", "Profile name", qt.QLineEdit__Normal, "", &accepted))
	if !accepted || name == "" {
		return
	}
	cfg := w.readConfig()
	if _, exists := cfg.Profiles[name]; exists {
		w.setNotice(fmt.Sprintf("Profile “%s” already exists.", name), true)
		return
	}
	cfg.ActiveProfile = name
	cfg.Profiles[name] = config.Profile{Lighting: cfg.Lighting, Knobs: cfg.Knobs}
	cfg.Normalize()
	if err := w.controller.UpdateConfig(cfg); err != nil {
		w.setNotice("Could not create profile: "+err.Error(), true)
		return
	}
	w.load(cfg)
	w.setNotice(fmt.Sprintf("Profile “%s” created.", name), false)
}

func (w *Window) updateLightingVisibility() {
	mode := optionValue(w.mode, lightingOptions)
	w.reactiveWrap.SetVisible(mode == "vu" || mode == "spectrum")
	if mode == "spectrum" {
		w.visualHelp.SetText("The four RGB rings become independent frequency bands from bass to treble.")
	} else {
		w.visualHelp.SetText("The four RGB rings become a left-to-right level meter with four illuminated segments.")
	}
}

func (w *Window) updateKnobVisibility(index int) {
	k := &w.knobs[index]
	turn := optionValue(k.turnKind, turnOptions)
	needsTarget := turn == "app" || turn == "output_device" || turn == "input_device" || turn == "obs_input"
	k.turnTargetWrap.SetVisible(needsTarget)
	k.percentWrap.SetVisible(turn != "none" && turn != "obs_filter" && turn != "shell")
	k.filterWrap.SetVisible(turn == "obs_filter")
	k.shellTurnWrap.SetVisible(turn == "shell")
	press := optionValue(k.pressKind, pressOptions)
	k.pressTargetWrap.SetVisible(press == "obs_scene" || press == "obs_toggle_input_mute" || press == "profile" || press == "application")
	k.shellPressWrap.SetVisible(press == "shell")
}

func (w *Window) chooseColor(button *qt.QPushButton, value *string) {
	initial := qt.NewQColor6(*value)
	selected := qt.QColorDialog_GetColor3(initial, w.QWidget, "Choose RGB color")
	if selected != nil && selected.IsValid() {
		*value = selected.Name()
		paintColorButton(button, *value)
	}
}

func (w *Window) startStatusTimer() {
	w.timer = qt.NewQTimer()
	w.timer.OnTimeout(func() {
		current := w.controller.Config()
		if !w.loading && current.ActiveProfile != w.cfg.ActiveProfile {
			w.load(current)
			w.setNotice(fmt.Sprintf("Profile “%s” activated.", current.ActiveProfile), false)
		}
		devStatus := w.device.Status()
		status := w.engine.Status()
		if w.preview {
			w.deviceStatus.SetText("Interface preview · hardware disabled")
			w.deviceStatus.SetStyleSheet("color: #9fbfff; font-weight: 700; padding: 8px 12px;")
		} else if devStatus.Connected {
			name := devStatus.Device
			if name == "" {
				name = "PCPanel connected"
			}
			w.deviceStatus.SetText(name)
			w.deviceStatus.SetStyleSheet("color: #62dda7; font-weight: 700; padding: 8px 12px;")
		} else if devStatus.Error != "" {
			w.deviceStatus.SetText("Device error")
			w.deviceStatus.SetToolTip(devStatus.Error)
			w.deviceStatus.SetStyleSheet("color: #ff7e8d; font-weight: 700; padding: 8px 12px;")
		} else {
			w.deviceStatus.SetText("PCPanel disconnected")
			w.deviceStatus.SetStyleSheet("color: #f2b66d; font-weight: 700; padding: 8px 12px;")
		}

		mode := w.cfg.Lighting.Mode
		for index, raw := range status.Values {
			k := &w.knobs[index]
			if !k.dialDragging {
				k.dial.SetValue(raw)
			}
			k.value.SetText(fmt.Sprintf("%d · %d%%", raw, (raw*100+127)/255))
			level := float64(raw) / 255
			if mode == "vu" {
				level = clamp(status.VULevel*4 - float64(index))
			} else if mode == "spectrum" {
				level = clamp(status.Spectrum[index])
			}
			w.levels[index].SetValue(int(level*100 + 0.5))
		}
		text := status.LastEvent
		if text == "" {
			text = "No events yet"
		}
		if mode == "vu" {
			text = fmt.Sprintf("Level visualizer · %d%%", int(status.VULevel*100+0.5))
		} else if mode == "spectrum" {
			text = fmt.Sprintf("Spectrum · %d · %d · %d · %d%%", int(status.Spectrum[0]*100), int(status.Spectrum[1]*100), int(status.Spectrum[2]*100), int(status.Spectrum[3]*100))
		}
		if status.LastError != "" {
			text = "Action: " + status.LastError
		}
		if status.VUError != "" {
			text = "Visualizer: " + status.VUError
		}
		if audioError := w.audio.LastError(); audioError != "" {
			text += " · Audio: " + audioError
		}
		w.diagnostics.SetText(text)
	})
	w.timer.Start(250)
}

func (w *Window) refreshTargets() {
	go func() {
		apps, appErr := w.audio.Apps(true)
		devices, deviceErr := w.audio.Devices(true)
		desktopApplications, desktopErr := desktopapps.Discover()
		mainthread.Start(func() {
			if appErr == nil {
				seen := make(map[string]bool)
				w.apps = w.apps[:0]
				for _, app := range apps {
					if !seen[app.Name] {
						seen[app.Name] = true
						w.apps = append(w.apps, app.Name)
					}
				}
				sort.Strings(w.apps)
			}
			if deviceErr == nil {
				w.outputDevices = nil
				w.inputDevices = nil
				for _, item := range devices {
					entry := choice{label: item.Name, value: item.ID}
					if item.Kind == "output" {
						w.outputDevices = append(w.outputDevices, entry)
					} else if item.Kind == "input" {
						w.inputDevices = append(w.inputDevices, entry)
					}
				}
			}
			if desktopErr == nil {
				w.setDesktopApplications(desktopApplications)
			}
			w.populateAllTargets()
		})
	}()
	w.refreshOBSInputs()
}

func (w *Window) setDesktopApplications(applications []desktopapps.Application) {
	counts := make(map[string]int)
	for _, application := range applications {
		counts[application.Name]++
	}
	w.desktopApps = nil
	for _, application := range applications {
		label := application.Name
		if counts[application.Name] > 1 {
			label += " · " + strings.TrimSuffix(application.DesktopID, ".desktop")
		}
		w.desktopApps = append(w.desktopApps, choice{label: label, value: application.Path})
	}
}

func (w *Window) refreshOBSInputs() {
	go func() {
		inputs, err := w.engine.OBSInputs()
		if err != nil {
			return
		}
		sort.Strings(inputs)
		mainthread.Start(func() {
			w.obsInputs = inputs
			w.populateAllTargets()
		})
	}()
}

func (w *Window) refreshOBSFilters(index int) {
	if w.loading || optionValue(w.knobs[index].turnKind, turnOptions) != "obs_filter" {
		return
	}
	source := strings.TrimSpace(w.knobs[index].filterSource.CurrentText())
	if source == "" {
		return
	}
	go func() {
		filters, err := w.engine.OBSFilters(source)
		if err != nil {
			return
		}
		mainthread.Start(func() { replaceEditable(w.knobs[index].filterName, filters) })
	}()
}

func (w *Window) populateAllTargets() {
	w.populateVUTarget()
	for index := range w.knobs {
		w.populateTurnTarget(index)
		w.populatePressTarget(index)
		replaceEditable(w.knobs[index].filterSource, w.obsInputs)
	}
}

func (w *Window) populateVUTarget() {
	kind := optionValue(w.vuSource, sourceOptions)
	switch kind {
	case "app":
		replaceEditable(w.vuTarget, w.apps)
	case "output_device":
		replaceEditableChoices(w.vuTarget, w.outputDevices)
	case "input_device":
		replaceEditableChoices(w.vuTarget, w.inputDevices)
	default:
		replaceEditable(w.vuTarget, nil)
	}
}

func (w *Window) populateTurnTarget(index int) {
	k := &w.knobs[index]
	switch optionValue(k.turnKind, turnOptions) {
	case "app":
		replaceEditable(k.turnTarget, w.apps)
	case "output_device":
		replaceEditableChoices(k.turnTarget, w.outputDevices)
	case "input_device":
		replaceEditableChoices(k.turnTarget, w.inputDevices)
	case "obs_input":
		replaceEditable(k.turnTarget, w.obsInputs)
	default:
		replaceEditable(k.turnTarget, nil)
	}
}

func (w *Window) populatePressTarget(index int) {
	k := &w.knobs[index]
	switch optionValue(k.pressKind, pressOptions) {
	case "profile":
		k.pressTarget.SetEditable(true)
		replaceEditable(k.pressTarget, w.cfg.ProfileNames())
	case "obs_toggle_input_mute":
		k.pressTarget.SetEditable(true)
		replaceEditable(k.pressTarget, w.obsInputs)
	case "application":
		replaceNamedChoices(k.pressTarget, w.desktopApps)
	default:
		k.pressTarget.SetEditable(true)
		replaceEditable(k.pressTarget, nil)
	}
}

func (w *Window) setNotice(message string, isError bool) {
	w.notice.SetText(message)
	if isError {
		w.notice.SetStyleSheet("color: #ff7e8d;")
	} else {
		w.notice.SetStyleSheet("color: #62dda7;")
	}
}

func labeled(title string, field *qt.QWidget) *qt.QWidget {
	container := qt.NewQWidget2()
	layout := qt.NewQVBoxLayout2()
	layout.SetContentsMargins(0, 0, 0, 0)
	layout.SetSpacing(3)
	label := qt.NewQLabel3(title)
	label.SetStyleSheet("color: #aebbd0;")
	layout.AddWidget(label.QWidget)
	layout.AddWidget(field)
	container.SetLayout(layout.QLayout)
	return container
}

func editableCombo(placeholder string) *qt.QComboBox {
	combo := qt.NewQComboBox2()
	combo.SetEditable(true)
	combo.LineEdit().SetPlaceholderText(placeholder)
	return combo
}

func newOptionsCombo(options []option) *qt.QComboBox {
	combo := qt.NewQComboBox2()
	for _, item := range options {
		combo.AddItem(item.label)
	}
	return combo
}

func optionValue(combo *qt.QComboBox, options []option) string {
	text := combo.CurrentText()
	for _, item := range options {
		if item.label == text {
			return item.value
		}
	}
	return ""
}

func setOptionValue(combo *qt.QComboBox, options []option, value string) {
	for _, item := range options {
		if item.value == value {
			combo.SetCurrentText(item.label)
			return
		}
	}
}

func newSpin(minimum, maximum int, suffix string) *qt.QSpinBox {
	spin := qt.NewQSpinBox2()
	spin.SetRange(minimum, maximum)
	spin.SetSuffix(suffix)
	return spin
}

func newDouble(minimum, maximum float64, decimals int) *qt.QDoubleSpinBox {
	spin := qt.NewQDoubleSpinBox2()
	spin.SetRange(minimum, maximum)
	spin.SetDecimals(decimals)
	return spin
}

func paintColorButton(button *qt.QPushButton, color string) {
	button.SetText(strings.ToUpper(color))
	button.SetStyleSheet(fmt.Sprintf("background: %s; color: %s; border: 1px solid #607087;", color, contrastColor(color)))
}

func contrastColor(color string) string {
	var red, green, blue int
	if _, err := fmt.Sscanf(color, "#%02x%02x%02x", &red, &green, &blue); err != nil {
		return "#ffffff"
	}
	if red*299+green*587+blue*114 > 150000 {
		return "#10151c"
	}
	return "#ffffff"
}

func replaceEditable(combo *qt.QComboBox, values []string) {
	current := comboValue(combo)
	combo.Clear()
	combo.AddItems(values)
	combo.SetCurrentText(current)
}

func replaceEditableChoices(combo *qt.QComboBox, choices []choice) {
	current := comboValue(combo)
	combo.Clear()
	for _, item := range choices {
		label := item.label
		if label != item.value {
			label += " · " + item.value
		}
		combo.AddItem3(label, qt.NewQVariant11(item.value))
	}
	index := combo.FindData(qt.NewQVariant11(current))
	if index >= 0 {
		combo.SetCurrentIndex(index)
	} else {
		combo.SetCurrentText(current)
	}
}

func replaceNamedChoices(combo *qt.QComboBox, choices []choice) {
	current := comboValue(combo)
	combo.SetEditable(false)
	combo.Clear()
	for _, item := range choices {
		combo.AddItem3(item.label, qt.NewQVariant11(item.value))
	}
	index := combo.FindData(qt.NewQVariant11(current))
	if index >= 0 {
		combo.SetCurrentIndex(index)
	} else if current != "" {
		combo.AddItem3("Unavailable application", qt.NewQVariant11(current))
		combo.SetCurrentIndex(combo.Count() - 1)
	} else if len(choices) == 0 {
		combo.AddItem("No desktop applications found")
	}
}

func comboValue(combo *qt.QComboBox) string {
	if data := combo.CurrentData(); data != nil {
		if value := strings.TrimSpace(data.ToString()); value != "" {
			return value
		}
	}
	return strings.TrimSpace(combo.CurrentText())
}

func clamp(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
