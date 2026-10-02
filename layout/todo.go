// Package layout calculates monitor regions independently of Windows.
package layout

import "image"

// Split reserves a strip along side, leaving the rest for other windows.
// The caller validates side and fraction. Rounding preserves at least one pixel
// in each region when the monitor dimension permits it.
func Split(work image.Rectangle, side string, fraction float64) (reserved, available image.Rectangle) {
	reserved, available = work, work
	size := work.Dx()
	if side == "top" || side == "bottom" {
		size = work.Dy()
	}
	n := int(float64(size) * fraction)
	if n < 1 {
		n = 1
	}
	if n >= size {
		n = size - 1
	}
	switch side {
	case "left":
		reserved.Max.X = work.Min.X + n
		available.Min.X = reserved.Max.X
	case "right":
		reserved.Min.X = work.Max.X - n
		available.Max.X = reserved.Min.X
	case "top":
		reserved.Max.Y = work.Min.Y + n
		available.Min.Y = reserved.Max.Y
	case "bottom":
		reserved.Min.Y = work.Max.Y - n
		available.Max.Y = reserved.Min.Y
	}
	return
}
