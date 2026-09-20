package pager

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type line struct {
	fixed         string
	scrollable    string
	lenFixed      int
	lenScrollable int
	lenTotal      int
}

func makeLine(fixed, scrollable string) line {
	l := line{
		fixed:         fixed,
		scrollable:    scrollable,
		lenFixed:      ansi.StringWidth(fixed),
		lenScrollable: ansi.StringWidth(scrollable),
	}

	l.lenTotal = l.lenFixed + l.lenScrollable

	return l
}

func (l line) clamp(xPos, maxWidth int) string {
	if maxWidth <= 0 || maxWidth >= l.lenTotal {
		return l.fixed + l.scrollable
	}

	buf := strings.Builder{}
	buf.WriteString(ansi.Cut(l.fixed, 0, min(l.lenFixed, maxWidth)))
	maxWidth -= l.lenFixed

	if maxWidth <= 0 {
		return buf.String()
	}

	xPos = min(xPos, l.lenScrollable-maxWidth)

	return l.fixed + ansi.Cut(l.scrollable, xPos, xPos+maxWidth)
}
