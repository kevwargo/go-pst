package tree

import (
	"bytes"
	"fmt"
	"slices"
)

type renderState struct {
	tree   *Tree
	lb     bytes.Buffer
	levels []nestLevel
}

type nestLevel struct {
	isLast bool
}

func (r *renderState) render(ps []*process) {
	ps = slices.DeleteFunc(ps, func(p *process) bool { return r.tree.filter.matches(p.id) == nil })

	if l := len(ps); l > 0 {
		for _, p := range ps[:l-1] {
			r.renderProcLine(p, false)
		}
		r.renderProcLine(ps[l-1], true)
	}
}

func (r *renderState) renderProcLine(p *process, isLast bool) {
	var branch rune

	for _, nl := range r.levels {
		if nl.isLast {
			branch = ' '
		} else {
			branch = lineVertical
		}
		fmt.Fprintf(&r.lb, "%c ", branch)
	}

	if isLast {
		branch = lineBranchLast
	} else {
		branch = lineBranchRight
	}

	fmt.Fprintf(&r.lb, "%c%c[%d] %s", branch, lineHorizontal, p.id, p.attrs.cmdline(r.tree.filter.matches(p.id)))
	r.tree.GetPager().WriteLine(r.lb.String(), "")
	r.lb.Reset()

	r.levels = append(r.levels, nestLevel{isLast: isLast})
	r.render(p.children)
	r.levels = r.levels[:len(r.levels)-1]
}

const (
	lineVertical    = '\u2502'
	lineHorizontal  = '\u2500'
	lineBranchRight = '\u251c'
	lineBranchLast  = '\u2514'
)
