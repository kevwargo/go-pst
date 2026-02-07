package tree

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kevwargo/go-pst/internal/benchmark"
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

type filterFn func(*process) bool

func (t *Tree) Filter(pattern string) {
	t.filter = &filter{
		fn:      t.initMatchFn(pattern),
		matches: make(map[int]matchType),
	}

	t.refreshMatches()
}

func (t *Tree) refreshMatches() {
	defer benchmark.Record("tree.refreshMatches", time.Now())

	// TODO: take dead into account

	clear(t.filter.matches)
	for _, p := range t.top {
		t.matchProcess(p)
	}

	t.refreshView()
}

func (t *Tree) matchProcess(p *process) {
	if p.exit != nil && !t.cfg.ShowDead {
		return
	}

	if t.filter.fn(p) {
		t.filter.matches[p.id] = matchDirect
		t.matchDescendants(p)
	} else {
		for _, c := range p.children {
			t.matchProcess(c)

			if t.filter.matches[c.id] != matchNone {
				t.filter.matches[p.id] = matchAsAncestor
			}
		}
	}
}

func (t *Tree) matchDescendants(p *process) {
	for _, c := range p.children {
		if t.filter.fn(c) {
			t.filter.matches[c.id] = matchDirect
		} else {
			t.filter.matches[c.id] = matchAsDescendant
		}

		t.matchDescendants(c)
	}
}

func (t *Tree) initMatchFn(pattern string) filterFn {
	if t.cfg.FullMatch {
		return func(p *process) bool {
			return strings.Contains(p.attrs.cmdline(), pattern)
		}
	}

	return func(p *process) bool {
		if strconv.Itoa(p.id) == pattern {
			// TODO: standardize this behavior, maybe with a separate flag
			return true
		}

		return slices.ContainsFunc(p.attrs.args, func(a string) bool {
			return strings.Contains(a, pattern)
		})
	}
}
