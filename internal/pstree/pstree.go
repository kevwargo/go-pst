package pstree

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
)

type Config struct {
	FullMatch         bool
	ShowThreads       bool
	ShowMainThread    bool
	ShowWorkdir       bool
	ShowUID           bool
	ShowGID           bool
	ShowBasicFDs      bool
	ShowProcessGroups bool
	Truncate          int
	Trace             bool
}

type Tree struct {
	processes []*process
	cfg       *Config
}

func Build(cfg *Config) (*Tree, error) {
	processes, err := collectProcesses(cfg)
	if err != nil {
		return nil, err
	}

	return &Tree{
		processes: processes,
		cfg:       cfg,
	}, nil
}

func (t *Tree) Print(pattern string) {
	state := matchState{
		level:             0,
		forceMatch:        false,
		matchedDirectly:   make(map[int]bool),
		matchedByChildren: make(map[int]bool),
	}

	if t.cfg.FullMatch {
		state.matchFn = func(p *process) bool {
			return strings.Contains(p.formatCmdline(), pattern)
		}
	} else {
		state.matchFn = func(p *process) bool {
			args := append([]string{strconv.Itoa(p.id), p.name}, p.args...)

			return slices.ContainsFunc(args, func(a string) bool {
				return strings.Contains(a, pattern)
			})
		}
	}

	if t.cfg.Trace {
		state.trace = new(trace)

		f, err := os.OpenFile("go-pst.trace.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			state.trace.log = os.Stderr
		} else {
			state.trace.log = f

			defer f.Close()
		}
	}

	for _, p := range t.processes {
		t.printProcess(p, state)
	}

	if t.cfg.Trace {
		fmt.Fprintf(state.trace.log, "Match cache hits: %d\n", state.trace.cacheHits)
	}
}

type matchState struct {
	level             int
	forceMatch        bool
	matchFn           func(*process) bool
	matchedDirectly   map[int]bool
	matchedByChildren map[int]bool
	trace             *trace
}

type trace struct {
	cacheHits int
	log       io.Writer
}

func (t *Tree) printProcess(p *process, state matchState) {
	state.forceMatch = matchDirectly(p, &state) || state.forceMatch

	if !matchByChildren(p, &state) && !state.forceMatch {
		return
	}

	fmt.Printf("%s%s\n", strings.Repeat(indent, state.level), p.format(t.cfg))

	if t.cfg.ShowThreads {
		for _, t := range p.threads {
			fmt.Printf(" %s%s\n", strings.Repeat(indent, state.level), t.format())
		}
	}

	state.level++

	for _, c := range p.children {
		t.printProcess(c, state)
	}
}

func matchDirectly(p *process, state *matchState) bool {
	matched, ok := state.matchedDirectly[p.id]
	if !ok {
		matched = state.matchFn(p)
		state.matchedDirectly[p.id] = matched
	}

	if state.trace != nil {
		if ok {
			state.trace.cacheHits++
		} else {
			fmt.Fprintf(state.trace.log, "(%v) dir: [%d] %s\n", matched, p.id, p.formatCmdline())
		}
	}

	return matched
}

func matchByChildren(p *process, state *matchState) bool {
	matched, ok := state.matchedByChildren[p.id]
	if ok {
		if state.trace != nil {
			state.trace.cacheHits++
		}
		return matched
	}

	for _, c := range p.children {
		if matched = matchDirectly(c, state) || matchByChildren(c, state); matched {
			break
		}
	}

	state.matchedByChildren[p.id] = matched

	if state.trace != nil {
		fmt.Fprintf(state.trace.log, "(%v) child: [%d] %s\n", matched, p.id, p.formatCmdline())
	}

	return matched
}

const (
	indent = "  "
)
