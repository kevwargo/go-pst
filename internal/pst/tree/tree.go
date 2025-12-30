package tree

import (
	"errors"
	"fmt"
	"iter"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/kevwargo/go-pst/internal/pager"
	"github.com/kevwargo/go-pst/internal/procwatch"
	"github.com/kevwargo/go-pst/internal/pst/proc"
)

type Config struct {
	PCfg          proc.Config
	FullMatch     bool
	ShowDead      bool
	Truncate      int
	FitTermWidth  bool
	FitTermHeight bool
}

type Tree struct {
	cfg    *Config
	pMap   map[int]*proc.Process
	pager  *pager.Pager
	top    []*proc.Process
	filter *filter
}

func Build(cfg *Config) (*Tree, error) {
	t := Tree{
		cfg: cfg,
	}

	if err := t.load(); err != nil {
		return nil, err
	}

	return &t, nil
}

func (t *Tree) All() iter.Seq[*proc.Process] {
	return maps.Values(t.pMap)
}

func (t *Tree) Get(pid int) *proc.Process {
	return t.pMap[pid]
}

func (t *Tree) View() string {
	return t.GetPager().View()
}

func (t *Tree) GetPager() *pager.Pager {
	if t.pager != nil {
		return t.pager
	}

	t.pager = new(pager.Pager)

	if !t.cfg.FitTermHeight && !t.cfg.FitTermWidth {
		t.pager.SetMaxWidth(t.cfg.Truncate)
	} else if w, h, err := term.GetSize(os.Stdout.Fd()); err == nil {
		t.pager.SetMaxWidth(w)
		t.pager.SetMaxHeight(h)
	}

	return t.pager
}

func (t *Tree) HandleNewProcess(ev procwatch.EventForkProc)   {}
func (t *Tree) HandleNewThread(ev procwatch.EventForkThread)  {}
func (t *Tree) HandleExec(ev procwatch.EventExec)             {}
func (t *Tree) HandleComm(ev procwatch.EventComm)             {}
func (t *Tree) HandleProcessExit(ev procwatch.EventExitProc)  {}
func (t *Tree) HandleThreadExit(ev procwatch.EventExitThread) {}

func (t *Tree) ToggleShowDead() {
	t.cfg.ShowDead = !t.cfg.ShowDead
	t.refreshMatches()
}

func (t *Tree) ToggleThreads() {
	t.cfg.PCfg.Threads = !t.cfg.PCfg.Threads
	t.reload()
}

func (t *Tree) CleanupDead() {
	for pid, p := range t.pMap {
		p.Children = slices.DeleteFunc(p.Children, func(c *proc.Process) bool {
			return c.Exit != nil
		})
		p.Threads = slices.DeleteFunc(p.Threads, func(t *proc.Thread) bool {
			return t.Dead
		})

		if p.Exit != nil {
			delete(t.pMap, pid)
		}
	}

	t.refreshMatches()
}

func (t *Tree) refreshView() {
	pg := t.GetPager()
	pg.Reset()

	for _, p := range t.top {
		t.renderProcess(p, pg, 0)
	}
}

func (t *Tree) renderProcess(p *proc.Process, pg *pager.Pager, level int) {
	if p.Exit != nil && !t.cfg.ShowDead {
		return
	}

	if t.filter != nil && t.filter.matches[p.ID] == matchNone {
		return
	}

	indent := strings.Repeat("  ", level)
	pg.WriteLine(fmt.Sprintf("%s[%d] ", indent, p.ID), p.Attrs.Cmdline())

	for _, c := range p.Children {
		t.renderProcess(c, pg, level+1)
	}
}

func (t *Tree) load() error {
	ps, err := proc.LoadAll(&t.cfg.PCfg)
	if err != nil {
		return err
	}

	t.pMap = make(map[int]*proc.Process, len(ps))
	for _, p := range ps {
		t.pMap[p.ID] = p
	}

	t.removeSelf()

	for _, p := range t.pMap {
		if p.ParentID <= 0 {
			t.top = append(t.top, p)
		} else if parent := t.pMap[p.ParentID]; parent != nil {
			parent.Children = append(parent.Children, p)
		}
	}

	return nil
}

func (t *Tree) reload() error {
	for _, p := range t.pMap {
		if err := p.Reload(&t.cfg.PCfg); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				delete(t.pMap, p.ID)
				continue
			}

			return err
		}
	}

	t.refreshMatches()

	return nil
}

func (t *Tree) removeSelf() {
	p := t.pMap[os.Getpid()]
	if p == nil {
		return
	}

	delete(t.pMap, p.ID)

	for parent := t.pMap[p.ParentID]; isSudoAncestor(parent, p); parent = t.pMap[parent.ParentID] {
		delete(t.pMap, parent.ID)
	}
}

func isSudoAncestor(ancestor, descendant *proc.Process) bool {
	if ancestor == nil {
		return false
	}

	if ancestor.Attrs.Name != "sudo" {
		return false
	}

	if len(ancestor.Attrs.Args) < 2 {
		return false
	}

	return slices.Equal(ancestor.Attrs.Args[1:], descendant.Attrs.Args)
}
