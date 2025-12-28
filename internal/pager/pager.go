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
	p.xPos++
	p.needsRefresh = true
}

func (p *Pager) Right() {
	p.xPos--
	p.needsRefresh = true
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

	lines := p.lines
	if p.maxHeight > 0 && len(lines) > p.maxHeight {
		lines = lines[p.yPos : p.yPos+p.maxHeight]
	}

	for i, line := range lines {
		textLine := line.fixed + line.scrollable
		if p.maxWidth > 0 && len(textLine) > p.maxWidth {
			textLine = textLine[:p.maxWidth]
		}

		if i == len(lines)-1 {
			fmt.Fprint(&p.buf, textLine)
		} else {
			fmt.Fprintln(&p.buf, textLine)
		}
	}

	p.needsRefresh = false
}

type line struct {
	fixed      string
	scrollable string
}
