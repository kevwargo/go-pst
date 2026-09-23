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
	p.lines = append(p.lines, makeLine(fixed, scrollable))
	p.needsRefresh = true
}

func (p *Pager) SetMaxWidth(w int) {
	p.maxWidth = w
	p.incXPos(0) // normalize X offset
	p.needsRefresh = true
}

func (p *Pager) SetMaxHeight(h int) {
	p.maxHeight = h
	p.incYPos(0) // normalize Y offset
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

func (p *Pager) Left(delta uint) {
	if p.incXPos(-int(delta)) {
		p.needsRefresh = true
	}
}

func (p *Pager) Right(delta uint) {
	if p.incXPos(int(delta)) {
		p.needsRefresh = true
	}
}

func (p *Pager) FullLeft() {
	if p.xPos != 0 {
		p.xPos = 0
		p.needsRefresh = true
	}
}

func (p *Pager) FullRight() {
	if xPosMax := p.xPosMax(); p.xPos != xPosMax {
		p.xPos = xPosMax
		p.needsRefresh = true
	}
}

func (p *Pager) Reset() {
	p.lines = p.lines[:0]
}

func (p *Pager) View() string {
	if len(p.lines) == 0 {
		return ""
	}

	if p.needsRefresh {
		p.refresh()
	}

	return p.buf.String()
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

func (p *Pager) incXPos(delta int) bool {
	if p.maxWidth <= 0 {
		return false
	}

	old := p.xPos
	p.xPos = max(p.xPos+delta, 0)
	p.xPos = min(p.xPos, p.xPosMax())

	return old != p.xPos
}

func (p *Pager) xPosMax() (xPosMax int) {
	for _, line := range p.visibleLines() {
		xPosMax = max(xPosMax, line.lenTotal-p.maxWidth)
	}

	return xPosMax
}

func (p *Pager) refresh() {
	p.buf.Reset()

	lines := p.visibleLines()
	if ll := len(lines); ll > 0 {
		for _, l := range lines[:ll-1] {
			p.renderLine(l, false)
		}
		p.renderLine(lines[ll-1], true)
	}

	p.needsRefresh = false
}

func (p *Pager) renderLine(l line, isLast bool) {
	textLine := l.clamp(p.xPos, p.maxWidth)
	if isLast {
		fmt.Fprint(&p.buf, textLine)
	} else {
		fmt.Fprintln(&p.buf, textLine)
	}
}

func (p *Pager) visibleLines() []line {
	if p.maxHeight > 0 && len(p.lines) > p.maxHeight {
		return p.lines[p.yPos : p.yPos+p.maxHeight]
	}

	return p.lines
}
