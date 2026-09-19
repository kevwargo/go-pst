package tree

import (
	"errors"
	"fmt"
	"log"
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

func (t *Tree) Reload() error {
	if err := t.load(); err != nil {
		return err
	}

	t.refreshMatches()

	return nil
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

	if t.cfg.PCfg.matchDebug(p) {
		log.Printf("exit %d: p:%p exit:%p", p.id, p, p.exit)
	}

	// When `p` is exited and properly reaped, this returns NotFound error and we just ignore it.
	// When `p` is a zombie, this succeeds and sets the state as Z which is later rendered.
	p.loadAttrs(&t.cfg.PCfg)

	p.children = slices.DeleteFunc(p.children, func(c *process) bool {
		if err := c.reload(&t.cfg.PCfg); err != nil {
			return false
		}

		if pp := t.pMap[c.parentID]; pp != nil && pp.id != p.id {
			pp.children = append(pp.children, c)
			return true
		}

		return false
	})

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

	t.sort(t.top)
	for _, p := range t.top {
		t.renderProcess(p, pg, 0)
	}
}

func (t *Tree) isProcVisible(p *process) bool {
	if p.exit != nil && !t.cfg.ShowDead {
		return false
	}

	return t.filter == nil || t.filter.matches[p.id] != noMatch
}

func (t *Tree) sort(ps []*process) int {
	weights := make(map[int]int)
	totalWeight := 0

	for _, p := range ps {
		if !t.isProcVisible(p) {
			continue
		}

		w := t.sort(p.children)
		weights[p.id] = w
		totalWeight += w + 1
	}

	slices.SortFunc(ps, func(a, b *process) int {
		if diff := weights[a.id] - weights[b.id]; diff != 0 {
			return diff
		}

		return a.id - b.id
	})

	return totalWeight
}

func (t *Tree) renderProcess(p *process, pg *pager.Pager, level int) {
	if !t.isProcVisible(p) {
		return
	}

	indent := strings.Repeat("  ", level)

	var exit string

	if p.attrs.isZombie() {
		exit = "Z"
	} else if p.exit != nil {
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
		ugid = fmt.Sprintf("[%s:%s] ", p.attrs.uid.ID(), p.attrs.gid.ID())
	}

	var pathEnv string
	if t.cfg.PCfg.PathEnv {
		pathEnv = strings.Join(p.attrs.pathEnvEntries, ":") + " "
	}

	var mem string
	if t.cfg.PCfg.MemoryUsage {
		mem = p.attrs.memUsage.render() + " "
	}

	pg.WriteLine(
		fmt.Sprintf("%s%s%s ", indent, pid, exit),
		fmt.Sprintf("%s%s%s%s%s", mem, pathEnv, ugid, workdir, p.attrs.cmdline()),
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
	newPMap, err := loadPMap(&t.cfg.PCfg)
	if err != nil {
		return err
	}

	if t.pMap == nil {
		t.pMap = newPMap
	} else {
		t.mergePMap(newPMap)
	}

	t.top = nil
	for _, p := range t.pMap {
		if p.parentID <= 0 {
			t.top = append(t.top, p)
		} else if pp := t.pMap[p.parentID]; pp != nil {
			if !slices.ContainsFunc(pp.children, func(c *process) bool {
				return c.id == p.id
			}) {
				pp.children = append(pp.children, p)
			}
		}
	}

	return nil
}

func (t *Tree) mergePMap(newPMap map[int]*process) {
	for pid, old := range t.pMap {
		p, ok := newPMap[pid]

		if t.cfg.PCfg.matchDebug(old) {
			log.Printf("merge %d: present:%v old:%p new:%p, exit:%p", pid, ok, old, p, old.exit)
		}

		if !ok {
			old.attrs.state = "" // reset zombie state
			if old.exit == nil {
				old.exit = &exitStatus{code: -1}
			}
		} else {
			t.pMap[pid] = p
			if old.exit != nil {
				if p.attrs.isZombie() {
					p.exit = old.exit
				} else {
					log.Printf("PID %d most probably recycled - old:%p new:%p", old, p)
				}
			}
		}
	}

	for pid, p := range newPMap {
		if _, ok := t.pMap[pid]; !ok {
			t.pMap[pid] = p
		}
	}
}

func (t *Tree) reload() error {
	for _, p := range t.pMap {
		if err := p.reload(&t.cfg.PCfg); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	t.refreshMatches()

	return nil
}
