package pstree

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
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
	ShowNamespacePID  bool
	Truncate          int

	Interactive bool
	ShowDead    bool
	Fullscreen  bool

	InspectAllFDs       bool
	DumpProcessSnapshot string
}

type Tree struct {
	cfg      *Config
	topLevel []*process
	pMap     map[int]*process
	matchFn  func(*process, string) bool
	showDead bool
}

func Build(cfg *Config) (*Tree, error) {
	tree := Tree{
		cfg:      cfg,
		showDead: cfg.ShowDead,
	}

	if err := tree.loadProcesses(); err != nil {
		return nil, err
	}

	tree.arrangeProcesses()
	tree.initMatchFn()

	return &tree, nil
}

func (t *Tree) loadProcesses() error {
	selfPid := os.Getpid()
	t.pMap = make(map[int]*process)

	return iterIntDirEntries(procDir, func(pid int) error {
		if pid == selfPid {
			return nil
		}

		p, err := loadProcess(pid, t.cfg)
		if err == nil {
			t.pMap[pid] = p
		} else if errors.Is(err, os.ErrNotExist) {
			err = nil
		}

		return err
	})
}

func (t *Tree) arrangeProcesses() {
	for _, p := range t.pMap {
		if p.parentID < 1 {
			t.topLevel = append(t.topLevel, p)
		} else if parent := t.pMap[p.parentID]; parent != nil {
			parent.children = append(parent.children, p)
		}
	}
}

func (t *Tree) initMatchFn() {
	if t.cfg.FullMatch {
		t.matchFn = func(p *process, pattern string) bool {
			return strings.Contains(p.attrs.formatCmdline(), pattern)
		}
	} else {
		t.matchFn = func(p *process, pattern string) bool {
			if strconv.Itoa(p.id) == pattern {
				return true
			}

			return slices.ContainsFunc(p.attrs.cmdlineArgs(), func(a string) bool {
				return strings.Contains(a, pattern)
			})
		}
	}
}

func (t *Tree) Run(pattern string) error {
	if t.cfg.DumpProcessSnapshot != "" {
		return t.dumpProcessSnapshot(pattern)
	}

	if t.cfg.InspectAllFDs {
		return t.inspectFDs()
	}

	t.match(pattern)

	if t.cfg.Interactive {
		return runTUI(t)
	}

	return t.renderStdout()
}

func (t *Tree) match(pattern string) {
	for _, p := range t.topLevel {
		t.matchProcess(p, pattern)
	}
}

func (t *Tree) matchProcess(p *process, pattern string) {
	if t.matchFn(p, pattern) {
		p.match = matchDirect
		t.matchDescendants(p, pattern)
	} else {
		p.match = matchNone

		for _, c := range p.children {
			t.matchProcess(c, pattern)

			if c.match != matchNone {
				p.match = matchAsAncestor
			}
		}
	}
}

func (t *Tree) matchDescendants(p *process, pattern string) {
	for _, c := range p.children {
		if t.matchFn(c, pattern) {
			c.match = matchDirect
		} else {
			c.match = matchAsDescendant
		}

		t.matchDescendants(c, pattern)
	}
}

func (t *Tree) insertProcess(pid, ppid int) error {
	if _, ok := t.pMap[pid]; ok {
		return nil
	}

	parent := t.pMap[ppid]
	if parent == nil {
		return fmt.Errorf("parent %d of new process %d not found", ppid, pid)
	}

	p := parent.fork(pid)
	parent.children = append(parent.children, p)
	t.pMap[pid] = p

	return nil
}

func (t *Tree) reloadProcess(pid int) error {
	old := t.pMap[pid]
	if old == nil {
		return fmt.Errorf("process %d not found", pid)
	}

	new, err := loadProcess(pid, t.cfg)
	if err != nil {
		return err
	}

	parent := t.pMap[new.parentID]
	if parent == nil {
		return fmt.Errorf("parent %d of new process %d not found", new.parentID, pid)
	}

	switch parent.match {
	case matchDirect, matchAsDescendant:
		new.match = matchAsDescendant
	}

	*old = *new

	return nil
}

func (t *Tree) removeProcess(pid, ppid, exitCode, signal int) error {
	p := t.pMap[pid]
	if p == nil {
		return fmt.Errorf("process %d not found", pid)
	}

	parent := t.pMap[ppid]
	if parent == nil {
		return fmt.Errorf("parent %d of new process %d not found", ppid, pid)
	}

	delete(t.pMap, pid)
	p.exit = &exitStatus{
		code:   exitCode,
		signal: signal,
	}

	return nil
}

func (t *Tree) toggleShowDead() {
	t.showDead = !t.showDead
}

func (t *Tree) renderStdout() error {
	t.render(os.Stdout)

	return nil
}

func (t *Tree) render(w io.Writer) {
	for _, p := range t.topLevel {
		t.dumpProcess(p, w, 0)
	}
}

func (t *Tree) dumpProcess(p *process, w io.Writer, level int) {
	if p.match == matchNone {
		return
	}

	if p.exit != nil && !t.showDead {
		return
	}

	indent := strings.Repeat("  ", level)

	fmt.Fprintf(w, "%s%s\n", indent, p.format(t.cfg))

	if t.cfg.ShowThreads {
		for _, t := range p.threads {
			fmt.Fprintf(w, " %s%s\n", indent, t.format())
		}
	}

	for _, c := range p.children {
		t.dumpProcess(c, w, level+1)
	}
}

func (t *Tree) inspectFDs() error {
	fdLinkMap := make(map[string]int)

	var visitProc func([]*process)
	visitProc = func(ps []*process) {
		for _, p := range ps {
			for _, link := range p.fds {
				fdLinkMap[link]++
			}

			visitProc(p.children)
		}
	}

	visitProc(t.topLevel)

	specialRe := regexp.MustCompile("^[a-zA-Z0-9_-]+:")
	fdLinks := slices.Collect(maps.Keys(fdLinkMap))
	slices.SortFunc(fdLinks, func(fd1, fd2 string) int {
		special1 := specialRe.MatchString(fd1)
		special2 := specialRe.MatchString(fd2)

		if special1 && !special2 {
			return -1
		}
		if special2 && !special1 {
			return 1
		}

		if diff := fdLinkMap[fd1] - fdLinkMap[fd2]; diff != 0 {
			return diff
		} else {
			return cmp.Compare(fd1, fd2)
		}
	})

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, fdLink := range fdLinks {
		fmt.Fprintf(tw, "%s\t%d\n", fdLink, fdLinkMap[fdLink])
	}

	return tw.Flush()
}

func (t *Tree) dumpProcessSnapshot(pattern string) error {
	pid, err := strconv.Atoi(pattern)
	if err != nil {
		return fmt.Errorf("%q is not a valid PID: %w", pattern, err)
	}

	p := t.pMap[pid]
	if p == nil {
		return fmt.Errorf("process with PID %d does not exist", pid)
	}

	return p.dumpSnapshot(t.cfg.DumpProcessSnapshot)
}
