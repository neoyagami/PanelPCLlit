//go:build qt

package qtui

const StyleSheet = `
QWidget {
  background: #11151b;
  color: #e8edf5;
  font-family: "Inter", "Noto Sans", sans-serif;
  font-size: 13px;
}
QLabel, QCheckBox { background: transparent; }
QMainWindow, QScrollArea, QScrollArea > QWidget > QWidget { background: #0c1015; }
QGroupBox {
  background: #161c24;
  border: 1px solid #2a3442;
  border-radius: 10px;
  margin-top: 12px;
  padding: 14px 10px 10px 10px;
  font-weight: 600;
}
QGroupBox::title { subcontrol-origin: margin; left: 12px; padding: 0 6px; color: #aebbd0; }
QLineEdit, QPlainTextEdit, QComboBox, QSpinBox, QDoubleSpinBox {
  background: #0e1319;
  border: 1px solid #344154;
  border-radius: 6px;
  padding: 6px;
  selection-background-color: #2b7fff;
}
QLineEdit:focus, QPlainTextEdit:focus, QComboBox:focus, QSpinBox:focus, QDoubleSpinBox:focus {
  border-color: #5b9cff;
}
QPushButton {
  background: #263244;
  border: 1px solid #3b4a60;
  border-radius: 7px;
  padding: 7px 12px;
  font-weight: 600;
}
QPushButton:hover { background: #314058; }
QPushButton:pressed { background: #1e2837; }
QPushButton#primary { background: #2874e8; border-color: #4d92fa; }
QPushButton#primary:hover { background: #3883ef; }
QLabel#title { font-size: 24px; font-weight: 700; color: #f7f9fc; }
QLabel#subtitle { color: #8998ae; }
QLabel#connected { color: #62dda7; font-weight: 700; }
QLabel#disconnected { color: #f2b66d; font-weight: 700; }
QLabel#error { color: #ff7e8d; font-weight: 700; }
QLabel#notice { color: #62dda7; }
QTabWidget::pane { border: 1px solid #2a3442; border-radius: 6px; top: -1px; }
QTabBar::tab { background: #10161e; padding: 7px 12px; border: 1px solid #2a3442; }
QTabBar::tab:selected { background: #263244; color: #ffffff; }
QProgressBar { border: 1px solid #344154; border-radius: 4px; background: #0e1319; text-align: center; }
QProgressBar::chunk { background: #4f95ff; border-radius: 3px; }
QDial { background: transparent; }
QToolTip { background: #263244; color: #ffffff; border: 1px solid #50617a; }
`
