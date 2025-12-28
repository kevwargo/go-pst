package pager

import (
	"bytes"
	"fmt"
)

type Pager struct {
	lines []line

	maxWidth  int
	maxHeight int
	yPos      int
	xPos      int

	buf          bytes.Buffer
	needsRefresh bool
}

func (p *Pager) WriteLine(fixed, scrollable string) {
	p.lines = append(p.lines, line{
		fixed:      fixed,
		scrollable: scrollable,
	})

	p.needsRefresh = true
}

func (p *Pager) SetMaxWidth(w int) {
	p.maxWidth = w
	p.needsRefresh = true
}

func (p *Pager) SetMaxHeight(h int) {
	p.maxHeight = h
	p.needsRefresh = true
}

func (p *Pager) Up() {
	if p.incYPos(-1) {
		p.needsRefresh = true
	}
}

func (p *Pager) Down() {
	if p.incYPos(1) {
		p.needsRefresh = true
	}
}

func (p *Pager) PageUp() {
	if p.incYPos(1 - p.maxHeight) {
		p.needsRefresh = true
	}
}

func (p *Pager) PageDown() {
	if p.incYPos(p.maxHeight - 1) {
		p.needsRefresh = true
	}
}

func (p *Pager) incYPos(delta int) bool {
	if p.maxHeight <= 0 || len(p.lines) <= p.maxHeight {
		return false
	}

	old := p.yPos
	p.yPos = max(p.yPos+delta, 0)
	p.yPos = min(p.yPos, len(p.lines)-p.maxHeight)

	return old != p.yPos
}

func (p *Pager) Left() {
	if p.incXPos(-1) {
		p.needsRefresh = true
	}
}

func (p *Pager) Right() {
	if p.incXPos(1) {
		p.needsRefresh = true
	}
}

func (p *Pager) incXPos(delta int) bool {
	if p.maxWidth <= 0 {
		return false
	}

	var xPosMax int
	for _, line := range p.visibleLines() {
		xPosMax = max(xPosMax, line.length()-p.maxWidth)
	}

	old := p.xPos
	p.xPos = max(p.xPos+delta, 0)
	p.xPos = min(p.xPos, xPosMax)

	return old != p.xPos
}

func (p *Pager) Reset() {
	p.lines = p.lines[:0]
}

func (p *Pager) String() string {
	if len(p.lines) == 0 {
		return ""
	}

	if p.needsRefresh {
		p.refresh()
	}

	return p.buf.String()
}

func (p *Pager) refresh() {
	p.buf.Reset()

	lines := p.visibleLines()
	for i, line := range lines {
		textLine := line.clamp(p.xPos, p.maxWidth)
		if i == len(lines)-1 {
			fmt.Fprint(&p.buf, textLine)
		} else {
			fmt.Fprintln(&p.buf, textLine)
		}
	}

	p.needsRefresh = false
}

func (p *Pager) visibleLines() []line {
	if p.maxHeight > 0 && len(p.lines) > p.maxHeight {
		return p.lines[p.yPos : p.yPos+p.maxHeight]
	}

	return p.lines
}

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
		if diff := len(l.fixed) + len(scrollable) - maxWidth; diff > 0 {
			xPos := min(xPos, diff)
			scrollable = scrollable[xPos : xPos+maxWidth-len(l.fixed)]
		}
	}

	return l.fixed + scrollable
}
