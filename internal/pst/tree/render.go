package tree

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

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
	var filtered []*process
	for _, p := range ps {
		if r.matchProc(p.id) != nil {
			filtered = append(filtered, p)
		}
	}

	if l := len(filtered); l > 0 {
		for _, p := range filtered[:l-1] {
			r.renderProcLine(p, false)
		}
		r.renderProcLine(filtered[l-1], true)
	}
}

func (r *renderState) renderProcLine(p *process, isLast bool) {
	fixed := r.renderFixed(p, isLast)
	scrollable := r.renderScrollable(p)
	r.pager.WriteLine(fixed, scrollable)

	r.levels = append(r.levels, nestLevel{isLast: isLast})
	r.render(p.children)
	r.levels = r.levels[:len(r.levels)-1]
}

func (r *renderState) renderFixed(p *process, isLast bool) string {
	r.renderControls(isLast)

	var pid string
	if p.attrs.nsPid == nil {
		pid = strconv.Itoa(p.id)
	} else {
		pid = strings.Join(p.attrs.nsPid, " ")
	}
	if r.matchProc(p.id).pid {
		pid = matchStyle.Styled(pid)
	}
	fmt.Fprintf(&r.buf, "[%s]", pid)

	if p.attrs.isZombie() {
		r.buf.WriteRune('Z')
	}
	if p.exit != nil {
		if p.exit.signal > 0 {
			fmt.Fprintf(&r.buf, "*s:%d*", p.exit.signal)
		} else {
			fmt.Fprintf(&r.buf, "*e:%d*", p.exit.code)
		}
	}

	r.buf.WriteRune(' ')

	res := r.buf.String()
	r.buf.Reset()

	return res
}

func (r *renderState) renderScrollable(p *process) string {
	fmt.Fprintf(&r.buf, "%s", p.attrs.cmdline(r.matchProc(p.id)))

	res := r.buf.String()
	r.buf.Reset()

	return res
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
