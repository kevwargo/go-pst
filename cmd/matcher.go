package cmd

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type matcher struct {
	direct   map[int]struct{}
	children map[int]struct{}
	fn       func(*process) bool
}

func newMatcher(pattern string, fullMatch bool) *matcher {
	m := matcher{
		direct:   make(map[int]struct{}),
		children: make(map[int]struct{}),
	}

	if fullMatch {
		m.fn = func(p *process) bool {
			return strings.Contains(p.renderCmdline(), pattern)
		}
	} else {
		m.fn = func(p *process) bool {
			args := []string{strconv.Itoa(p.ID), p.Name}
			args = append(args, p.Args...)

			return slices.ContainsFunc(args, func(a string) bool {
				return strings.Contains(a, pattern)
			})
		}
	}

	return &m
}

func (m *matcher) add(p *process) (res bool) {
	for _, c := range p.Children {
		if m.add(c) {
			m.children[p.ID] = struct{}{}
			res = true
		}
	}

	if m.fn(p) {
		m.direct[p.ID] = struct{}{}
		res = true
	}

	return res
}

func (m *matcher) printMatching(cfg *config, p *process, level int, force bool) {
	if !force {
		_, force = m.direct[p.ID]
	}

	_, childrenMatch := m.children[p.ID]

	if childrenMatch || force {
		fmt.Printf("%s%s\n", strings.Repeat("  ", level), p.render(cfg))
		for _, c := range p.Children {
			m.printMatching(cfg, c, level+1, force)
		}
	}
}
