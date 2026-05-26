package tui

// Layout holds pre-computed panel dimensions derived from terminal size.
// Call LayoutFor to construct one; all fields are read-only after construction.
//
// Panel geometry:
//
//	┌─ header (HeaderH rows) ────────────────────────────────────────────────┐
//	│ left column (LeftW)      │ right column (RightW)                        │
//	│  DeviceList (BodyH rows) │  ActiveJobs (ActiveH rows)                   │
//	│                          │  Errors     (ErrorsH rows)                   │
//	├──────────────────────────┴─────────────────────────────────────────────┤
//	│ OutputTail (OutputH rows)                                               │
//	└─────────────────────────────────────────────────────────────────────────┘
type Layout struct {
	TotalW  int // terminal total width
	TotalH  int // terminal total height
	HeaderH int // header row height (fixed: 3)
	OutputH int // output tail panel height (fixed: 6)
	BodyH   int // TotalH - HeaderH - OutputH (minimum 1)
	LeftW   int // device list column ≈ 60% of TotalW
	RightW  int // right column: TotalW - LeftW
	ActiveH int // active jobs panel height: BodyH / 2
	ErrorsH int // errors panel height: BodyH - ActiveH
}

// LayoutFor returns a Layout for a terminal of the given width and height.
func LayoutFor(width, height int) Layout {
	const headerH = 3
	const outputH = 6

	leftW := width * 60 / 100
	rightW := width - leftW

	bodyH := height - headerH - outputH
	if bodyH < 1 {
		bodyH = 1
	}

	activeH := bodyH / 2
	errorsH := bodyH - activeH

	return Layout{
		TotalW:  width,
		TotalH:  height,
		HeaderH: headerH,
		OutputH: outputH,
		BodyH:   bodyH,
		LeftW:   leftW,
		RightW:  rightW,
		ActiveH: activeH,
		ErrorsH: errorsH,
	}
}
