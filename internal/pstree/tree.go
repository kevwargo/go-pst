package pstree

import (
	"cmp"
	"errors"
	"fmt"
	"log"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/x/term"
	"github.com/kevwargo/go-pst/internal/pager"
	"github.com/kevwargo/go-pst/internal/procwatch"
)

type Config struct {
	Pattern *string

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
	matchFn  func(*process) bool
	pager    pager.Pager
}

func Build(cfg *Config) (*Tree, error) {
	if cfg.Pattern == nil && !cfg.InspectAllFDs {
		return nil, errors.New("Pattern required when --inspect-all-fds not used")
	}

	tree := Tree{cfg: cfg}

	if err := tree.loadProcesses(); err != nil {
		return nil, err
	}

	tree.arrangeProcesses()

	if cfg.Pattern != nil {
		tree.initMatchFn()
		tree.matchProcesses()
	}

	return &tree, nil
}

func (t *Tree) Run() error {
	if t.cfg.DumpProcessSnapshot != "" {
		return t.dumpProcessSnapshot()
	}

	if t.cfg.InspectAllFDs {
		return t.inspectFDs()
	}

	if err := t.initPager(); err != nil {
		return err
	}

	t.repaint()
	if t.cfg.Interactive {
		return runTUI(t)
	}

	_, err := fmt.Println(t.render())
	return err
}

func (t *Tree) loadProcesses() error {
	selfPid := os.Getpid()
	t.pMap = make(map[int]*process)

	return iterIntDirEntries(procDir, func(pid int) error {
		if pid == selfPid {
			// TODO: skip also sudo ancestors of selfPid, if present.
			// Example:
			//   [292436] /bin/bash
			//     [359788] sudo ./go-pst ipython -i
			//       [359794] sudo ./go-pst ipython -i
			// self -> [359795] ./go-pst ipython -i
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
		t.matchFn = func(p *process) bool {
			return strings.Contains(p.attrs.formatCmdline(), *t.cfg.Pattern)
		}
	} else {
		t.matchFn = func(p *process) bool {
			if strconv.Itoa(p.id) == *t.cfg.Pattern {
				// TODO: standardize this behavior, maybe with a separate flag
				return true
			}

			return slices.ContainsFunc(p.attrs.cmdlineArgs(), func(a string) bool {
				return strings.Contains(a, *t.cfg.Pattern)
			})
		}
	}
}

func (t *Tree) matchProcesses() {
	for _, p := range t.topLevel {
		t.matchProcess(p)
	}
}

func (t *Tree) matchProcess(p *process) {
	if t.matchFn(p) {
		p.match = matchDirect
		t.matchDescendants(p)
	} else {
		p.match = matchNone

		for _, c := range p.children {
			t.matchProcess(c)

			if c.match != matchNone {
				p.match = matchAsAncestor
			}
		}
	}
}

func (t *Tree) matchDescendants(p *process) {
	for _, c := range p.children {
		if t.matchFn(c) {
			c.match = matchDirect
		} else {
			c.match = matchAsDescendant
		}

		t.matchDescendants(c)
	}
}

func (t *Tree) resetPattern(pattern string) {
	if t.cfg.Pattern == nil {
		t.cfg.Pattern = &pattern
	} else {
		*t.cfg.Pattern = pattern
	}

	t.initMatchFn()
	t.matchProcesses()
}

func (t *Tree) handleNewProcess(ev procwatch.EventForkProc) {
	parent := t.pMap[ev.ParentPID]
	if parent == nil {
		return
	}

	p := parent.fork(ev.PID)
	parent.children = append(parent.children, p)
	t.pMap[p.id] = p

	switch p.match {
	case matchAsDescendant, matchDirect:
		t.repaint()
	}
}

func (t *Tree) handleNewThread(ev procwatch.EventForkThread) {
	if !t.cfg.ShowThreads {
		return
	}

	p := t.pMap[ev.PID]
	if p == nil {
		return
	}

	if p.loadThread(ev.TID) == nil {
		t.repaint()
	}
}

func (t *Tree) handleExec(ev procwatch.EventExec) {
	t.updateProcess(ev.PID)
}

func (t *Tree) handleComm(ev procwatch.EventComm) {
	if ev.PID == ev.TID {
		t.updateProcess(ev.PID)
	} else {
		if !t.cfg.ShowThreads {
			return
		}

		p := t.pMap[ev.PID]
		if p == nil {
			return
		}

		for _, thr := range p.threads {
			if thr.id == ev.TID && !thr.dead {
				thr.name = ev.Comm
				t.repaint()
				break
			}
		}
	}
}

func (t *Tree) updateProcess(pid int) {
	p := t.pMap[pid]
	if p == nil {
		return
	}

	newProc, err := loadProcess(pid, t.cfg)
	if err != nil {
		return
	}

	parent := t.pMap[newProc.parentID]
	if parent == nil {
		return
	}

	if p.match == matchDirect {
		log.Printf("matching %d|%q updating to %q", p.id, p.attrs.formatCmdline(), newProc.attrs.formatCmdline())
	}

	*p = *newProc

	t.matchProcesses()
	t.repaint()
}

func (t *Tree) handleProcessExit(ev procwatch.EventExitProc) {
	p := t.pMap[ev.PID]
	if p == nil {
		return
	}

	p.exit = &exitStatus{
		code:   ev.ExitCode,
		signal: ev.ExitSignal,
	}

	delete(t.pMap, ev.PID)

	for _, c := range p.children {
		nc, err := loadProcess(c.id, t.cfg)
		if err != nil {
			delete(t.pMap, c.id)
			continue
		}

		parent := t.pMap[nc.parentID] // new parent
		if parent == nil {
			delete(t.pMap, c.id)
			continue
		}

		log.Printf("reparent %d|%q %d -> %d", c.id, c.attrs.formatCmdline(), ev.PID, nc.parentID)

		parent.children = append(parent.children, c)
		*c = *nc
	}

	t.matchProcesses()
	t.repaint()
}

func (t *Tree) handleThreadExit(ev procwatch.EventExitThread) {
	p := t.pMap[ev.PID]
	if p == nil {
		return
	}

	for _, thr := range p.threads {
		if thr.id == ev.TID {
			thr.dead = true
			t.repaint()
			break
		}
	}
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

func (t *Tree) initPager() error {
	if t.cfg.Interactive || t.cfg.TruncateTerm {
		w, h, err := term.GetSize(os.Stdout.Fd())
		if err != nil {
			return fmt.Errorf("getting term size: %w", err)
		}

		t.pager.SetMaxWidth(w)
		if t.cfg.Interactive {
			t.pager.SetMaxHeight(h)
		}
	} else if t.cfg.Truncate > 0 {
		t.pager.SetMaxWidth(t.cfg.Truncate)
	}

	return nil
}

func (t *Tree) repaint() {
	t.pager.Reset()

	for _, p := range t.topLevel {
		p.render(t.cfg, &t.pager, 0)
	}
}

func (t *Tree) render() string {
	return t.pager.String()
}

func (t *Tree) inspectFDs() error {
	fdLinkMap := make(map[string]int)
	for _, p := range t.pMap {
		for _, link := range p.fds {
			fdLinkMap[link]++
		}
	}

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

func (t *Tree) dumpProcessSnapshot() error {
	if t.cfg.Pattern == nil {
		return errors.New("Pattern required for --dump-process-snapshot")
	}

	pid, err := strconv.Atoi(*t.cfg.Pattern)
	if err != nil {
		return fmt.Errorf("%q is not a valid PID: %w", *t.cfg.Pattern, err)
	}

	p := t.pMap[pid]
	if p == nil {
		return fmt.Errorf("process with PID %d does not exist", pid)
	}

	return p.dumpSnapshot(t.cfg.DumpProcessSnapshot)
}
