package pstree

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/x/term"
	"github.com/kevwargo/go-pst/internal/pager"
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
	TruncateTerm      bool

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
}

func Build(cfg *Config) (*Tree, error) {
	tree := Tree{cfg: cfg}

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
				// TODO: standardize this behavior, maybe with a separate flag
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

	pg, err := t.preparePager()
	if err != nil {
		return err
	}
	t.render(pg)

	if t.cfg.Interactive {
		return runTUI(t, pg)
	}

	_, err = fmt.Println(pg.String())
	return err
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

func (t *Tree) insertProcess(pid, ppid int) (bool, error) {
	if _, ok := t.pMap[pid]; ok {
		return false, nil
	}

	parent := t.pMap[ppid]
	if parent == nil {
		return false, fmt.Errorf("parent %d of new process %d not found", ppid, pid)
	}

	p := parent.fork(pid)
	parent.children = append(parent.children, p)
	t.pMap[pid] = p

	return parent.match == matchAsDescendant || parent.match == matchDirect, nil
}

func (t *Tree) reloadProcess(pid int) (bool, error) {
	old := t.pMap[pid]
	if old == nil {
		return false, fmt.Errorf("process %d not found", pid)
	}

	new, err := loadProcess(pid, t.cfg)
	if err != nil {
		return false, err
	}

	parent := t.pMap[new.parentID]
	if parent == nil {
		return false, fmt.Errorf("parent %d of new process %d not found", new.parentID, pid)
	}

	// TODO: match new process and optionally re-match existing processes that are related

	switch parent.match {
	case matchDirect, matchAsDescendant:
		new.match = matchAsDescendant
	}

	*old = *new

	return new.match == matchAsDescendant, nil
}

func (t *Tree) removeProcess(pid, ppid, exitCode, signal int) (bool, error) {
	p := t.pMap[pid]
	if p == nil {
		return false, fmt.Errorf("process %d not found", pid)
	}

	parent := t.pMap[ppid]
	if parent == nil {
		return false, fmt.Errorf("parent %d of new process %d not found", ppid, pid)
	}

	delete(t.pMap, pid)
	p.exit = &exitStatus{
		code:   exitCode,
		signal: signal,
	}

	return p.match != matchNone, nil
}

func (t *Tree) insertThread(tid, pid int) (bool, error) {
	if !t.cfg.ShowThreads {
		return false, nil
	}

	p := t.pMap[pid]
	if p == nil {
		return false, fmt.Errorf("process %d not found", pid)
	}

	for _, thread := range p.threads {
		if thread.id == tid {
			return false, nil
		}
	}

	if err := p.loadThread(tid); err != nil {
		return false, err
	}

	return true, nil
}

func (t *Tree) removeThread(tid, pid int) (bool, error) {
	p := t.pMap[pid]
	if p == nil {
		return false, fmt.Errorf("process %d not found", pid)
	}

	for _, t := range p.threads {
		if !t.dead && t.id == tid {
			t.dead = true

			return true, nil
		}
	}

	return false, nil
}

func (t *Tree) cleanupDead() (changed bool) {
	for pid, p := range t.pMap {
		p.children = slices.DeleteFunc(p.children, func(c *process) bool {
			if c.exit != nil {
				changed = true
				return true
			}

			return false
		})
		p.threads = slices.DeleteFunc(p.threads, func(t *thread) bool {
			if t.dead {
				changed = true
				return true
			}

			return false
		})
		if p.exit != nil {
			delete(t.pMap, pid)
		}
	}

	return changed
}

func (t *Tree) preparePager() (*pager.Pager, error) {
	var p pager.Pager

	if t.cfg.Interactive || t.cfg.TruncateTerm {
		w, h, err := term.GetSize(os.Stdout.Fd())
		if err != nil {
			return nil, fmt.Errorf("getting term size: %w", err)
		}

		p.SetMaxWidth(w)
		if t.cfg.Interactive {
			p.SetMaxHeight(h)
		}
	} else if t.cfg.Truncate > 0 {
		p.SetMaxWidth(t.cfg.Truncate)
	}

	return &p, nil
}

func (t *Tree) render(pg *pager.Pager) {
	pg.Reset()

	for _, p := range t.topLevel {
		p.render(t.cfg, pg, 0)
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
