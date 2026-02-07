package tree

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/kevwargo/go-pst/internal/benchmark"
	"github.com/kevwargo/go-pst/internal/pager"
	"github.com/kevwargo/go-pst/internal/procwatch"
)

type Config struct {
	PCfg          ProcConfig
	FullMatch     bool
	ShowDead      bool
	Truncate      int
	FitTermWidth  bool
	FitTermHeight bool
}

type Tree struct {
	cfg    *Config
	pMap   map[int]*process
	pager  *pager.Pager
	top    []*process
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
		if t.cfg.FitTermHeight {
			t.pager.SetMaxHeight(h)
		}
		if t.cfg.FitTermWidth {
			t.pager.SetMaxWidth(w)
		}
	}

	return t.pager
}

func (t *Tree) HandleNewProcess(ev procwatch.EventForkProc) {
	if parent := t.pMap[ev.ParentPID]; parent != nil {
		t.pMap[ev.PID] = parent.fork(ev.PID)
		t.refreshMatches()
	}
}

func (t *Tree) HandleNewThread(ev procwatch.EventForkThread) {
	if t.cfg.PCfg.Threads {
		if p := t.pMap[ev.PID]; p != nil {
			p.loadThread(ev.TID)
			t.refreshMatches()
		}
	}
}

func (t *Tree) HandleExec(ev procwatch.EventExec) {
	if p := t.pMap[ev.PID]; p != nil {
		p.reload(&t.cfg.PCfg)
		t.refreshMatches()
	}
}

func (t *Tree) HandleComm(ev procwatch.EventComm) {
	p := t.pMap[ev.PID]
	if p == nil {
		return
	}

	if ev.PID == ev.TID {
		p.reload(&t.cfg.PCfg)
		t.refreshMatches()
	} else if t.cfg.PCfg.Threads {
		for _, thr := range p.threads {
			if thr.id == ev.TID && !thr.dead {
				thr.name = ev.Comm
				t.refreshView()
				break
			}
		}
	}
}

func (t *Tree) HandleProcessExit(ev procwatch.EventExitProc) {
	p := t.pMap[ev.PID]
	if p == nil {
		return
	}

	p.exit = &exitStatus{
		code:   ev.ExitCode,
		signal: ev.ExitSignal,
	}

	delete(t.pMap, p.id)

	for _, c := range p.children {
		if c.reload(&t.cfg.PCfg) != nil {
			continue
		}

		newParent := t.pMap[c.parentID]
		if newParent == nil {
			continue
		}

		newParent.children = append(newParent.children, c)
	}

	t.refreshMatches()
}

func (t *Tree) HandleThreadExit(ev procwatch.EventExitThread) {
	p := t.pMap[ev.PID]
	if p == nil {
		return
	}

	for _, thr := range p.threads {
		if thr.id == ev.TID {
			thr.dead = true
			t.refreshView()
			break
		}
	}
}

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
		p.children = slices.DeleteFunc(p.children, func(c *process) bool {
			return c.exit != nil
		})
		p.threads = slices.DeleteFunc(p.threads, func(t *thread) bool {
			return t.dead
		})

		if p.exit != nil {
			delete(t.pMap, pid)
		}
	}

	t.refreshMatches()
}

func (t *Tree) refreshView() {
	defer benchmark.Record("tree.refreshView", time.Now())

	pg := t.GetPager()
	pg.Reset()

	// TODO: sort by (descendant-count; PID) pair

	for _, p := range t.top {
		t.renderProcess(p, pg, 0)
	}
}

func (t *Tree) renderProcess(p *process, pg *pager.Pager, level int) {
	if p.exit != nil && !t.cfg.ShowDead || t.filter != nil && t.filter.matches[p.id] == matchNone {
		return
	}

	indent := strings.Repeat("  ", level)

	var exit string
	if p.exit != nil {
		if p.exit.signal > 0 {
			exit = fmt.Sprintf("*s:%d*", p.exit.signal)
		} else {
			exit = fmt.Sprintf("*e:%d*", p.exit.code)
		}
	}

	var pid string
	if p.attrs.nsPid == nil {
		pid = fmt.Sprintf("[%d]", p.id)
	} else {
		pid = fmt.Sprint(p.attrs.nsPid)
	}

	var workdir string
	if t.cfg.PCfg.Workdir {
		workdir = fmt.Sprintf("{%s} ", p.attrs.workdir)
	}

	var ugid string
	if t.cfg.PCfg.UGID {
		ugid = fmt.Sprintf("[%s:%s] ", p.attrs.uid.id(), p.attrs.gid.id())
	}

	pg.WriteLine(
		fmt.Sprintf("%s%s%s ", indent, pid, exit),
		fmt.Sprintf("%s%s%s", ugid, workdir, p.attrs.cmdline()),
	)
	t.renderThreads(p, pg, indent)

	if t.cfg.PCfg.FDs {
		for _, fd := range p.fds {
			pg.WriteLine(fmt.Sprintf("%s %d -> ", indent, fd.num), fd.link)
		}
	}

	for _, c := range p.children {
		t.renderProcess(c, pg, level+1)
	}
}

func (t *Tree) renderThreads(p *process, pg *pager.Pager, indent string) {
	if !t.cfg.PCfg.Threads {
		return
	}

	for _, thr := range p.threads {
		var dead string
		if thr.dead {
			if !t.cfg.ShowDead {
				continue
			}

			dead = " *dead*"
		}

		pg.WriteLine(fmt.Sprintf("%s {%d%s} ", indent, thr.id, dead), thr.name)
	}
}

func (t *Tree) load() error {
	t.pMap = make(map[int]*process)

	for pid, err := range intDirEntries(procRoot) {
		if err != nil {
			return err
		}

		p, err := loadProc(pid, &t.cfg.PCfg)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}

			return err
		}

		t.pMap[p.id] = p
	}

	t.removeSelf()

	for _, p := range t.pMap {
		if p.parentID <= 0 {
			t.top = append(t.top, p)
		} else if parent := t.pMap[p.parentID]; parent != nil {
			parent.children = append(parent.children, p)
		}
	}

	return nil
}

func (t *Tree) reload() error {
	for _, p := range t.pMap {
		if err := p.reload(&t.cfg.PCfg); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				delete(t.pMap, p.id)
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

	delete(t.pMap, p.id)

	for parent := t.pMap[p.parentID]; isSudoAncestor(parent, p); parent = t.pMap[parent.parentID] {
		delete(t.pMap, parent.id)
	}
}

func isSudoAncestor(ancestor, descendant *process) bool {
	if ancestor == nil {
		return false
	}

	if ancestor.attrs.name != "sudo" {
		return false
	}

	if len(ancestor.attrs.args) < 2 {
		return false
	}

	return slices.Equal(ancestor.attrs.args[1:], descendant.attrs.args)
}
