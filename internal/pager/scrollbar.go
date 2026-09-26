package pager

import (
	"strings"

	"charm.land/lipgloss/v2"
)

type scrollRange struct {
	from int
	to   int
}

func (p *Pager) scrollRange() *scrollRange {
	if p.maxHeight <= 0 || p.maxHeight >= len(p.lines) {
		return nil
	}

	return &scrollRange{
		from: p.yPos,
		to:   p.yPos + p.maxHeight,
	}
}

func (p *Pager) renderScrollbar(r scrollRange) {
	ll := r.to - r.from
	scrollBar := make([]string, ll)
	scrollBar[0] = "^"
	scrollBar[ll-1] = "v"

	fromPercent := (r.from * 100) / len(p.lines)
	toPercent := fromPercent + ((r.to-r.from)*100)/len(p.lines)

	for i := range ll - 2 {
		percent := (i * 100) / (ll - 2)
		if percent >= fromPercent && percent < toPercent {
			scrollBar[i+1] = scrollBlock
		} else {
			scrollBar[i+1] = scrollBackground
		}
	}

	if r.from > 0 {
		scrollBar[1] = scrollBackground
	}
	if r.to < len(p.lines) {
		scrollBar[ll-2] = scrollBackground
	}

	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		p.buf.String(),
		" ",
		strings.Join(scrollBar, "\n"),
	)
	p.buf.Reset()
	p.buf.WriteString(strings.TrimRight(content, "\n "))
}

const (
	scrollBackground = "\u2591"
	scrollBlock      = "\u2588"
)
