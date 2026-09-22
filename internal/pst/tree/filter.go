package tree

import (
	"regexp"
	"strconv"
)

func (t *Tree) Filter(pattern string) error {
	if t.cfg.IgnoreCase {
		pattern = "(?i)" + pattern
	}

	rx, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}

	t.filter = &filter{
		tree:     t,
		pattern:  pattern,
		rx:       rx,
		matchMap: make(map[int]*match),
	}

	t.refreshMatches()

	return nil
}

type filter struct {
	tree     *Tree
	pattern  string
	rx       *regexp.Regexp
	matchMap map[int]*match
}

type match struct {
	pid     bool
	regions []region
}

type region struct {
	from int
	to   int
}

func (f *filter) matches(pid int) *match {
	if f == nil {
		return &defaultMatch
	}

	return f.matchMap[pid]
}

func (f *filter) refresh(ps []*process) {
	clear(f.matchMap)
	for _, p := range ps {
		f.matchProc(p)
	}
}

func (f *filter) apply(p *process) *match {
	if !f.tree.cfg.ShowDead && p.exit != nil {
		return nil
	}

	var m match
	if strconv.Itoa(p.id) == f.pattern {
		m.pid = true
	}
	for _, g := range f.rx.FindAllStringIndex(p.attrs.cmdline(nil), -1) {
		m.regions = append(m.regions, region{
			from: g[0],
			to:   g[1],
		})
	}

	if !m.pid && len(m.regions) == 0 {
		return nil
	}

	return &m
}

func (f *filter) matchProc(p *process) {
	if m := f.apply(p); m != nil {
		f.matchMap[p.id] = m
		f.matchAllDescendants(p.children)
	} else {
		for _, c := range p.children {
			f.matchProc(c)
			if f.matchMap[c.id] != nil {
				f.matchMap[p.id] = &defaultMatch
			}
		}
	}
}

func (f *filter) matchAllDescendants(children []*process) {
	for _, c := range children {
		if !f.tree.cfg.ShowDead && c.exit != nil {
			continue
		}

		m := f.apply(c)
		if m == nil {
			m = &defaultMatch
		}

		f.matchMap[c.id] = m
		f.matchAllDescendants(c.children)
	}
}

func (t *Tree) refreshMatches() {
	t.filter.refresh(t.top)
	t.refreshView()
}

var defaultMatch match
