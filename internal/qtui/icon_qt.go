//go:build qt

package qtui

import qt "github.com/mappu/miqt/qt6"

// panelIcon draws a small PCPanel silhouette into the binary. This keeps the
// window and tray icon available even when the executable is run directly.
func panelIcon() *qt.QIcon {
	pixmap := qt.NewQPixmap2(256, 256)
	pixmap.FillWithFillColor(qt.NewQColor11(0, 0, 0, 0))
	painter := qt.NewQPainter2(pixmap.QPaintDevice)
	painter.SetRenderHint2(qt.QPainter__Antialiasing, true)
	painter.SetPenWithStyle(qt.NoPen)

	setBrush := func(color string) {
		painter.SetBrush(qt.NewQBrush3(qt.NewQColor6(color)))
	}
	setBrush("#536174")
	painter.DrawRoundedRect2(8, 50, 240, 156, 25, 25)
	setBrush("#151b24")
	painter.DrawRoundedRect2(14, 56, 228, 144, 20, 20)
	setBrush("#222b37")
	painter.DrawRoundedRect2(23, 69, 210, 118, 14, 14)

	colors := []string{"#ff4d67", "#ffc857", "#35d0ba", "#7c83ff"}
	for index, color := range colors {
		x := 27 + index*53
		setBrush(color)
		painter.DrawEllipse2(x, 96, 42, 42)
		setBrush("#0c1118")
		painter.DrawEllipse2(x+9, 105, 24, 24)
		setBrush("#dce5f2")
		painter.DrawEllipse2(x+19, 108, 4, 9)
	}
	_ = painter.End()
	painter.Delete()
	icon := qt.NewQIcon2(pixmap)
	pixmap.Delete()
	return icon
}
