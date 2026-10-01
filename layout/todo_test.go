package layout

import (
	"image"
	"testing"
)

func TestSplit(t *testing.T) {
	work := image.Rect(-1600, 40, 0, 1040)
	for _, tc := range []struct {
		side                string
		reserved, available image.Rectangle
	}{
		{"left", image.Rect(-1600, 40, -1200, 1040), image.Rect(-1200, 40, 0, 1040)},
		{"right", image.Rect(-400, 40, 0, 1040), image.Rect(-1600, 40, -400, 1040)},
		{"top", image.Rect(-1600, 40, 0, 290), image.Rect(-1600, 290, 0, 1040)},
		{"bottom", image.Rect(-1600, 790, 0, 1040), image.Rect(-1600, 40, 0, 790)},
	} {
		a, b := Split(work, tc.side, .25)
		if a != tc.reserved || b != tc.available {
			t.Errorf("%s: got %v, %v; want %v, %v", tc.side, a, b, tc.reserved, tc.available)
		}
	}
}

func TestSplitPartitionsOddSizedMonitor(t *testing.T) {
	work := image.Rect(33, -1001, 1954, 0)
	for _, side := range []string{"left", "right", "top", "bottom"} {
		for _, fraction := range []float64{.000001, .25, .4, .999999} {
			a, b := Split(work, side, fraction)
			if a.Empty() || b.Empty() || !a.In(work) || !b.In(work) || a.Overlaps(b) ||
				a.Dx()*a.Dy()+b.Dx()*b.Dy() != work.Dx()*work.Dy() {
				t.Errorf("%s %v: invalid partition: %v, %v", side, fraction, a, b)
			}
		}
	}
}
