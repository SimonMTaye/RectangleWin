package main

import (
	"fmt"
	"testing"

	"github.com/ahmetb/RectangleWin/config"
	"github.com/gonutz/w32/v2"
)

func TestStackedVisibleFramesDoNotOverlap(t *testing.T) {
	for _, work := range []w32.RECT{
		{Right: 1920, Bottom: 1040},
		{Left: 1920, Right: 4480, Bottom: 1400},
		{Left: -2560, Top: -1440, Right: 0, Bottom: -41},
	} {
		// Border widths vary with display scaling; the top border is often zero.
		for _, border := range []int32{0, 7, 10, 14} {
			t.Run(fmt.Sprintf("%v/border%d", work, border), func(t *testing.T) {
				frame := w32.RECT{Left: work.Left + 100, Top: work.Top + 150, Right: work.Left + 900, Bottom: work.Top + 750}
				window := w32.RECT{Left: frame.Left - border, Top: frame.Top, Right: frame.Right + border, Bottom: frame.Bottom + border}
				for _, pair := range [][2]resizeFunc{
					{topHalf, bottomHalf},
					{topOneThirds, bottomTwoThirds},
					{topTwoThirds, bottomOneThirds},
					{topLeftHalf, bottomLeftHalf},
					{topRightHalf, bottomRightHalf},
				} {
					var visible [2]w32.RECT
					for i, snap := range pair {
						target := snap(work, frame)
						outer := windowBoundsForFrame(target, window, frame)
						visible[i] = w32.RECT{Left: outer.Left + border, Top: outer.Top, Right: outer.Right - border, Bottom: outer.Bottom - border}
						if visible[i] != target {
							t.Fatalf("visible frame = %v, want %v", visible[i], target)
						}
						// Snapping an already snapped window must not grow or shift it.
						if again := windowBoundsForFrame(snap(work, visible[i]), outer, visible[i]); again != outer {
							t.Fatalf("repeated snap moved window: %v -> %v", outer, again)
						}
					}
					gap := visible[1].Top - visible[0].Bottom
					if gap < 0 || gap > 1 {
						t.Errorf("stacked frames overlap or leave a gap: %v", visible)
					}
				}
			})
		}
	}
}

func TestTodoRemainingAreaActions(t *testing.T) {
	work := w32.RECT{Left: 0, Top: 0, Right: 1920, Bottom: 1080}
	reserved, available := splitTodo(work, config.Todo{Side: "left", Fraction: .25})
	if reserved.Right != 480 || available.Left != 480 {
		t.Fatalf("unexpected TODO split: %v, %v", reserved, available)
	}
	for i, f := range []resizeFunc{leftOneThirds, middleWidthThird, rightOneThirds} {
		got := f(available, work)
		want := w32.RECT{Left: int32(480 + i*480), Right: int32(960 + i*480), Bottom: 1080}
		if got != want {
			t.Errorf("third %d: got %v, want %v", i, got, want)
		}
	}
	if got := fillArea(available, work); got != available {
		t.Errorf("maximize: got %v, want %v", got, available)
	}
	if got := center(available, work); got != available {
		t.Errorf("oversized centered window: got %v, want %v", got, available)
	}
}

func TestThirdsFillWorkArea(t *testing.T) {
	for _, work := range []w32.RECT{
		{Left: 0, Top: 0, Right: 1920, Bottom: 1080},
		{Left: -1279, Top: 17, Right: 0, Bottom: 1038},
	} {
		left := leftOneThirds(work, w32.RECT{})
		middleWidth := middleWidthThird(work, w32.RECT{})
		right := rightOneThirds(work, w32.RECT{})
		if left.Left != work.Left || left.Right != middleWidth.Left || middleWidth.Right != right.Left || right.Right != work.Right {
			t.Errorf("horizontal thirds do not cover %#v: %#v %#v %#v", work, left, middleWidth, right)
		}
		for _, zone := range []w32.RECT{left, middleWidth, right} {
			if zone.Top != work.Top || zone.Bottom != work.Bottom || zone.Width() <= 0 {
				t.Errorf("horizontal third does not fit work area %#v: %#v", work, zone)
			}
		}

		top := topOneThirds(work, w32.RECT{})
		middleHeight := middleHeightThird(work, w32.RECT{})
		bottom := bottomOneThirds(work, w32.RECT{})
		if top.Top != work.Top || top.Bottom != middleHeight.Top || middleHeight.Bottom != bottom.Top || bottom.Bottom != work.Bottom {
			t.Errorf("vertical thirds do not cover %#v: %#v %#v %#v", work, top, middleHeight, bottom)
		}
		for _, zone := range []w32.RECT{top, middleHeight, bottom} {
			if zone.Left != work.Left || zone.Right != work.Right || zone.Height() <= 0 {
				t.Errorf("vertical third does not fit work area %#v: %#v", work, zone)
			}
		}
	}
}

func TestHalvesResizeBothDimensions(t *testing.T) {
	work := w32.RECT{Left: -101, Top: 35, Right: 1100, Bottom: 836}
	current := w32.RECT{Left: 400, Top: 400, Right: 500, Bottom: 500}
	for _, f := range []resizeFunc{leftHalf, rightHalf} {
		zone := f(work, current)
		if zone.Height() != work.Height() || zone.Width() != work.Width()/2 {
			t.Errorf("horizontal half did not resize: %#v", zone)
		}
	}
	for _, f := range []resizeFunc{topHalf, bottomHalf} {
		zone := f(work, current)
		if zone.Width() != work.Width() || zone.Height() != work.Height()/2 {
			t.Errorf("vertical half did not resize: %#v", zone)
		}
	}
}
