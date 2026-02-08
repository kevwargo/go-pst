package pager

type line struct {
	fixed      string
	scrollable string
}

func (l line) length() int {
	return len(l.fixed) + len(l.scrollable)
}

func (l line) clamp(xPos, maxWidth int) string {
	scrollable := l.scrollable
	if maxWidth > 0 {
		if diff := l.length() - maxWidth; diff > 0 {
			xPos := min(xPos, diff)
			scrollable = scrollable[xPos : xPos+maxWidth-len(l.fixed)]
		}
	}

	return l.fixed + scrollable
}
