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
	cfg       *Config
	buf       bytes.Buffer
	levels    []nestLevel
}

type nestLevel struct {
	isLastSibling bool
}

func (r *renderState) render(ps []*process) {
	r.renderFiltered(r.filter(ps))
}

func (r *renderState) filter(ps []*process) (filtered []*process) {
	for _, p := range ps {
		if r.matchProc(p.id) != nil {
			filtered = append(filtered, p)
		}
	}

	return filtered
}

func (r *renderState) renderFiltered(ps []*process) {
	for i, p := range ps {
		r.renderProcLine(p, i == len(ps)-1)
	}
}

func (r *renderState) renderProcLine(p *process, isLast bool) {
	r.pager.WriteLine(r.buildFixed(p, isLast), r.buildScrollable(p))

	r.levels = append(r.levels, nestLevel{isLastSibling: isLast})

	children := r.filter(p.children)
	r.renderThreads(p, len(children) > 0)
	r.renderFDs(p, len(children) > 0)
	r.renderFiltered(children)

	r.levels = r.levels[:len(r.levels)-1]
}

func (r *renderState) buildFixed(p *process, isLast bool) string {
	r.renderTreeLines(ltProcess, isLast, false)

	var pid string
	if p.attrs.nsPid == nil {
		pid = strconv.Itoa(p.id)
	} else {
		// TODO: match one of these PIDs and colorize only it
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

func (r *renderState) buildScrollable(p *process) string {
	r.renderMemUsage(p)
	r.renderPathEnv(p)
	r.renderUGID(p)
	r.renderWorkdir(p)
	r.renderCmdline(p)

	res := r.buf.String()
	r.buf.Reset()

	return res
}

func (r *renderState) renderMemUsage(p *process) {
	if r.cfg.PCfg.MemoryUsage {
		p.attrs.memUsage.renderTo(&r.buf)
		r.buf.WriteRune(' ')
	}
}

func (r *renderState) renderPathEnv(p *process) {
	if r.cfg.PCfg.PathEnv {
		le := len(p.attrs.pathEnvEntries)
		for i, e := range p.attrs.pathEnvEntries {
			r.buf.WriteString(e)
			if i == le-1 {
				r.buf.WriteRune(' ')
			} else {
				r.buf.WriteRune(':')
			}
		}
	}
}

func (r *renderState) renderUGID(p *process) {
	if r.cfg.PCfg.UGID {
		fmt.Fprintf(&r.buf, "[%s:%s] ", p.attrs.uid.ID(), p.attrs.gid.ID())
	}
}

func (r *renderState) renderWorkdir(p *process) {
	if r.cfg.PCfg.Workdir {
		fmt.Fprintf(&r.buf, "{%s} ", p.attrs.workdir)
	}
}

func (r *renderState) renderCmdline(p *process) {
	fmt.Fprintf(&r.buf, "%s", p.attrs.cmdline(r.matchProc(p.id)))
}

func (r *renderState) renderThreads(p *process, hasChildren bool) {
	if !r.cfg.PCfg.Threads {
		return
	}
	if r.matchProc(p.id).isEphemeral() && !r.cfg.PCfg.EphemeralStats {
		return
	}

	var threads []*thread
	for _, t := range p.threads {
		if !t.dead || r.cfg.ShowDead {
			threads = append(threads, t)
		}
	}
	if l := len(threads); l > 0 {
		for _, t := range threads[:l-1] {
			r.renderThreadLine(t, false, hasChildren)
		}
		r.renderThreadLine(threads[l-1], true, hasChildren)
	}
}

func (r *renderState) renderThreadLine(t *thread, isLast, hasChildren bool) {
	r.renderTreeLines(ltThread, isLast, hasChildren)
	fmt.Fprintf(&r.buf, "{%d", t.id)
	if t.dead {
		r.buf.WriteString(" *dead*")
	}
	r.buf.WriteString("} ")

	r.pager.WriteLine(r.buf.String(), t.name)
	r.buf.Reset()
}

func (r *renderState) renderFDs(p *process, hasChildren bool) {
	if !r.cfg.PCfg.FDs {
		return
	}
	if r.matchProc(p.id).isEphemeral() && !r.cfg.PCfg.EphemeralStats {
		return
	}

	for _, fd := range p.fds {
		r.renderTreeLines(ltFD, false, hasChildren)
		fmt.Fprintf(&r.buf, "%d -> ", fd.num)
		r.pager.WriteLine(r.buf.String(), fd.link)
		r.buf.Reset()
	}
}

type lineType int

const (
	ltProcess lineType = iota
	ltThread
	ltFD
)

func (r *renderState) renderTreeLines(lt lineType, isLast, extraVertical bool) {
	if len(r.levels) == 0 {
		return
	}

	for _, nl := range r.levels[1:] {
		if nl.isLastSibling {
			r.buf.WriteString("  ")
		} else {
			fmt.Fprintf(&r.buf, "%c ", lineVertical)
		}
	}

	switch lt {
	case ltProcess:
		if isLast {
			r.buf.WriteRune(lineBranchLast)
		} else {
			r.buf.WriteRune(lineBranchRight)
		}
		r.buf.WriteRune(lineHorizontal)
	case ltThread:
		if extraVertical {
			r.buf.WriteRune(lineVertical)
		} else {
			r.buf.WriteRune(' ')
		}

		if isLast {
			r.buf.WriteRune(lineBranchLast)
		} else {
			r.buf.WriteRune(lineBranchRight)
		}
	case ltFD:
		if extraVertical {
			r.buf.WriteRune(lineVertical)
		} else {
			r.buf.WriteRune(' ')
		}

		r.buf.WriteRune(' ')
	}
}

const (
	lineVertical    = '\u2502'
	lineHorizontal  = '\u2500'
	lineBranchRight = '\u251c'
	lineBranchLast  = '\u2514'
)
