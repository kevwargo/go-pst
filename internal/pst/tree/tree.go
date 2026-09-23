package tree

import (
	"errors"
	"log"
	"os"
	"slices"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/kevwargo/go-pst/internal/benchmark"
	"github.com/kevwargo/go-pst/internal/pager"
	"github.com/kevwargo/go-pst/internal/procwatch"
)

type Config struct {
	PCfg          ProcConfig
	IgnoreCase    bool
	ShowDead      bool
	Truncate      int
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

	if w, h, err := term.GetSize(os.Stdout.Fd()); err == nil {
		if t.cfg.FitTermHeight {
			t.pager.SetMaxHeight(h)
		}
		t.pager.SetMaxWidth(max(w, t.cfg.Truncate))
	} else {
		t.pager.SetMaxWidth(t.cfg.Truncate)
	}

	return t.pager
}

func (t *Tree) HandleNewProcess(ev procwatch.EventForkProc) {
	if parent := t.pMap[ev.ParentPID]; parent != nil {
		// FIXME: load from procfs (+ check if parentID didn't change due to race)
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

	r := renderState{
		matchProc: t.filter.matches,
		pager:     t.GetPager(),
		cfg:       t.cfg,
	}
	r.render(t.top)
}

func (t *Tree) sort(ps []*process) int {
	weights := make(map[int]int)
	totalWeight := 0

	for _, p := range ps {
		if t.filter.matches(p.id) == nil {
			continue
		}

		w := t.sort(p.children)
		weights[p.id] = w
		totalWeight += w + 1

		slices.SortFunc(p.threads, func(a, b *thread) int {
			return a.id - b.id
		})
	}

	slices.SortFunc(ps, func(a, b *process) int {
		if diff := weights[a.id] - weights[b.id]; diff != 0 {
			return diff
		}

		return a.id - b.id
	})

	return totalWeight
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
					log.Printf("PID %d most probably recycled - old:%p new:%p", pid, old, p)
				}
			}
			for _, t := range old.threads {
				if t.dead {
					p.threads = append(p.threads, t)
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

var matchStyle = ansi.NewStyle(ansi.AttrBrightRedForegroundColor, ansi.AttrBold)
