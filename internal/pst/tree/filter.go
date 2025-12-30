package tree

import (
	"slices"
	"strconv"
	"strings"

	"github.com/kevwargo/go-pst/internal/pst/proc"
)

type filter struct {
	fn      filterFn
	matches map[int]matchType
}

type matchType int

const (
	matchNone matchType = iota
	matchDirect
	matchAsDescendant
	matchAsAncestor
)

type filterFn func(*proc.Process) bool

func (t *Tree) Filter(pattern string) {
	t.filter = &filter{
		fn:      t.initMatchFn(pattern),
		matches: make(map[int]matchType),
	}

	t.refreshMatches()
}

func (t *Tree) refreshMatches() {
	// TODO: take dead into account

	for _, p := range t.top {
		t.matchProcess(p)
	}

	t.refreshView()
}

func (t *Tree) matchProcess(p *proc.Process) {
	if t.filter.fn(p) {
		t.filter.matches[p.ID] = matchDirect
		t.matchDescendants(p)
	} else {
		for _, c := range p.Children {
			t.matchProcess(c)

			if t.filter.matches[c.ID] != matchNone {
				t.filter.matches[p.ID] = matchAsAncestor
			}
		}
	}
}

func (t *Tree) matchDescendants(p *proc.Process) {
	for _, c := range p.Children {
		if t.filter.fn(c) {
			t.filter.matches[c.ID] = matchDirect
		} else {
			t.filter.matches[c.ID] = matchAsDescendant
		}

		t.matchDescendants(c)
	}
}

func (t *Tree) initMatchFn(pattern string) filterFn {
	if t.cfg.FullMatch {
		return func(p *proc.Process) bool {
			return strings.Contains(p.Attrs.Cmdline(), pattern)
		}
	}

	return func(p *proc.Process) bool {
		if strconv.Itoa(p.ID) == pattern {
			// TODO: standardize this behavior, maybe with a separate flag
			return true
		}

		return slices.ContainsFunc(p.Attrs.Args, func(a string) bool {
			return strings.Contains(a, pattern)
		})
	}
}
