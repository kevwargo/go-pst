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

	buf *bytes.Buffer
}

func (p *Pager) WriteLine(fixed, scrollable string) {
	p.lines = append(p.lines, line{
		fixed:      fixed,
		scrollable: scrollable,
	})
}

func (p *Pager) SetMaxWidth(w int) {
	p.maxWidth = w
}

func (p *Pager) SetMaxHeight(h int) {
	p.maxHeight = h
}

func (p *Pager) Up() {
	p.yPos--
}

func (p *Pager) Down() {
	p.yPos++
}

func (p *Pager) Left() {
	p.xPos++
}

func (p *Pager) Right() {
	p.xPos--
}

func (p *Pager) Reset() {
	p.lines = p.lines[:0]
}

func (p *Pager) String() string {
	if len(p.lines) == 0 {
		return ""
	}

	if p.buf == nil {
		var n int
		for _, l := range p.lines {
			n += len(l.fixed) + len(l.scrollable)
		}

		p.buf = bytes.NewBuffer(make([]byte, 0, n))
	}

	p.render()

	return p.buf.String()
}

func (p *Pager) render() {
	// TODO: make this optional to avoid unnecessary re-renders

	p.buf.Reset()

	lines := p.lines
	if p.maxHeight > 0 && len(lines) > p.maxHeight {
		p.yPos = max(p.yPos, 0)
		p.yPos = min(p.yPos, len(lines)-p.maxHeight)
		lines = lines[p.yPos : p.yPos+p.maxHeight]
	}

	for i, line := range lines {
		textLine := line.fixed + line.scrollable
		if p.maxWidth > 0 && len(textLine) > p.maxWidth {
			textLine = textLine[:p.maxWidth]
		}

		if i == len(lines)-1 {
			fmt.Fprint(p.buf, textLine)
		} else {
			fmt.Fprintln(p.buf, textLine)
		}
	}
}

type line struct {
	fixed      string
	scrollable string
}
