package tree

import (
	"regexp"
	"strconv"
	"time"

	"github.com/kevwargo/go-pst/internal/benchmark"
)

type filter struct {
	apply   filterFn
	matches map[int]matchType
}

type matchType int

const (
	noMatch matchType = iota
	matchDirect
	matchAsDescendant
	matchAsAncestor
)

type filterFn func(*process) bool

func (t *Tree) Filter(pattern string) error {
	rx, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}

	t.filter = &filter{
		apply: func(p *process) bool {
			return strconv.Itoa(p.id) == pattern || rx.MatchString(p.attrs.cmdline())
		},
		matches: make(map[int]matchType),
	}

	t.refreshMatches()

	return nil
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

	if t.filter.apply(p) {
		t.filter.matches[p.id] = matchDirect
		t.matchDescendants(p)
	} else {
		for _, c := range p.children {
			t.matchProcess(c)

			if t.filter.matches[c.id] != noMatch {
				t.filter.matches[p.id] = matchAsAncestor
			}
		}
	}
}

func (t *Tree) matchDescendants(p *process) {
	for _, c := range p.children {
		if t.filter.apply(c) {
			t.filter.matches[c.id] = matchDirect
		} else {
			t.filter.matches[c.id] = matchAsDescendant
		}

		t.matchDescendants(c)
	}
}
