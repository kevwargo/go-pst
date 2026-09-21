package tree

import (
	"bytes"
	"fmt"
	"slices"

	"github.com/kevwargo/go-pst/internal/pager"
)

type renderState struct {
	matchProc func(int) *match
	pager     *pager.Pager
	buf       bytes.Buffer
	levels    []nestLevel
}

type nestLevel struct {
	isLast bool
}

func (r *renderState) render(ps []*process) {
	ps = slices.DeleteFunc(ps, func(p *process) bool { return r.matchProc(p.id) == nil })

	if l := len(ps); l > 0 {
		for _, p := range ps[:l-1] {
			r.renderProcLine(p, false)
		}
		r.renderProcLine(ps[l-1], true)
	}
}

func (r *renderState) renderProcLine(p *process, isLast bool) {
	r.renderControls(isLast)
	fmt.Fprintf(&r.buf, "[%d] %s", p.id, p.attrs.cmdline(r.matchProc(p.id)))

	r.pager.WriteLine(r.buf.String(), "")
	r.buf.Reset()

	r.levels = append(r.levels, nestLevel{isLast: isLast})
	r.render(p.children)
	r.levels = r.levels[:len(r.levels)-1]
}

func (r *renderState) renderControls(isLast bool) {
	if len(r.levels) == 0 {
		return
	}

	var c rune

	for _, nl := range r.levels[1:] {
		if nl.isLast {
			c = ' '
		} else {
			c = lineVertical
		}
		fmt.Fprintf(&r.buf, "%c ", c)
	}

	if isLast {
		c = lineBranchLast
	} else {
		c = lineBranchRight
	}

	fmt.Fprintf(&r.buf, "%c%c", c, lineHorizontal)
}

const (
	lineVertical    = '\u2502'
	lineHorizontal  = '\u2500'
	lineBranchRight = '\u251c'
	lineBranchLast  = '\u2514'
)
