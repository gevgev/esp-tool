package tui

import "testing"

func TestLayoutFor_WidthSplit(t *testing.T) {
	l := LayoutFor(120, 40)
	if l.LeftW+l.RightW != 120 {
		t.Errorf("LeftW(%d) + RightW(%d) must equal total width 120", l.LeftW, l.RightW)
	}
}

func TestLayoutFor_LeftSixtyPercent(t *testing.T) {
	l := LayoutFor(100, 40)
	if l.LeftW != 60 {
		t.Errorf("want LeftW=60 (60%% of 100), got %d", l.LeftW)
	}
}

func TestLayoutFor_BodyHeight(t *testing.T) {
	l := LayoutFor(120, 40)
	want := 40 - l.HeaderH - l.OutputH
	if l.BodyH != want {
		t.Errorf("BodyH should be TotalH - HeaderH - OutputH = %d, got %d", want, l.BodyH)
	}
}

func TestLayoutFor_ActivePlusErrorsEqualsBody(t *testing.T) {
	l := LayoutFor(120, 40)
	if l.ActiveH+l.ErrorsH != l.BodyH {
		t.Errorf("ActiveH(%d) + ErrorsH(%d) must equal BodyH(%d)", l.ActiveH, l.ErrorsH, l.BodyH)
	}
}

func TestLayoutFor_HeaderHeightFixed3(t *testing.T) {
	l := LayoutFor(120, 40)
	if l.HeaderH != 3 {
		t.Errorf("HeaderH must be 3, got %d", l.HeaderH)
	}
}

func TestLayoutFor_MinBodyHeight(t *testing.T) {
	// Even with a very short terminal, BodyH must stay positive.
	l := LayoutFor(80, 10)
	if l.BodyH < 1 {
		t.Errorf("BodyH must be at least 1, got %d", l.BodyH)
	}
}

func TestLayoutFor_StoresTotalDimensions(t *testing.T) {
	l := LayoutFor(120, 40)
	if l.TotalW != 120 || l.TotalH != 40 {
		t.Errorf("want TotalW=120 TotalH=40, got %d×%d", l.TotalW, l.TotalH)
	}
}
