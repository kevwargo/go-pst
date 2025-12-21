package pstree

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/kevwargo/go-pst/internal/procwatch"
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
	InspectAllFDs     bool
	Interactive       bool
	DumpProcessImage  string
}

type Tree struct {
	topLevel []*process
	pMap     map[int]*process
	cfg      *Config
}

func Build(cfg *Config) (*Tree, error) {
	tree := &Tree{
		cfg:  cfg,
		pMap: make(map[int]*process),
	}
	selfPid := os.Getpid()

	err := iterIntDirEntries(procDir, func(pid int) error {
		if pid == selfPid {
			return nil
		}

		p, err := readProcess(pid, cfg)
		if err == nil {
			tree.pMap[pid] = p
		} else if errors.Is(err, os.ErrNotExist) {
			err = nil
		}

		return err
	})
	if err != nil {
		return nil, err
	}

	for _, p := range tree.pMap {
		if p.parentID < 1 {
			tree.topLevel = append(tree.topLevel, p)
		} else if parent := tree.pMap[p.parentID]; parent != nil {
			parent.children = append(parent.children, p)
		}
	}

	return tree, nil
}

func (t *Tree) Run(pattern string) error {
	if t.cfg.DumpProcessImage != "" {
		return t.dumpPID(pattern)
	}

	if t.cfg.InspectAllFDs {
		t.inspectFDs()
	} else {
		t.printMatching(pattern)

		if t.cfg.Interactive {
			return t.runInteractive()
		}
	}

	return nil
}

func (t *Tree) runInteractive() error {
	watcher, err := procwatch.Watch()
	if err != nil {
		return err
	}

	for {
		event, err := watcher.Recv()
		if err != nil || event == nil {
			return err
		}

		switch ev := event.(type) {
		case procwatch.EventFork:
			parent := strconv.Itoa(ev.ParentPID)
			if ev.ParentTID != ev.ParentPID {
				parent += fmt.Sprintf("(%d)", ev.ParentTID)
			}
			proc := strconv.Itoa(ev.PID)
			if ev.TID != ev.PID {
				proc += fmt.Sprintf("(%d)", ev.TID)
			}

			fmt.Printf("fork %s -> %s\n", parent, proc)
		case procwatch.EventExec:
			proc := strconv.Itoa(ev.PID)
			if ev.TID != ev.PID {
				proc += fmt.Sprintf("(%d)", ev.TID)
			}
			fmt.Printf("exec %s\n", proc)
		case procwatch.EventExit:
			parent := strconv.Itoa(ev.ParentPID)
			if ev.ParentTID != ev.ParentPID {
				parent += fmt.Sprintf("(%d)", ev.ParentTID)
			}
			proc := strconv.Itoa(ev.PID)
			if ev.TID != ev.PID {
				proc += fmt.Sprintf("(%d)", ev.TID)
			}

			fmt.Printf("exit %s, code:%d, signal:%d, parent:%s\n", proc, ev.ExitCode, ev.ExitSignal, parent)
		}
	}
}

func (t *Tree) printMatching(pattern string) {
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

	for _, p := range t.topLevel {
		t.printProcess(p, state)
	}
}

func (t *Tree) inspectFDs() {
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

	tw.Flush()
}

func (t *Tree) dumpPID(pattern string) error {
	pid, err := strconv.Atoi(pattern)
	if err != nil {
		return fmt.Errorf("%q is not a valid PID: %w", pattern, err)
	}

	p := t.pMap[pid]
	if p == nil {
		return fmt.Errorf("process with PID %d does not exist", pid)
	}

	return dump(t.cfg.DumpProcessImage, p)
}

func dump(dir string, p *process) error {
	err := filepath.WalkDir(pidPath(p.id), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		destPath := filepath.Join(dir, strings.TrimPrefix(path, procDir+"/"))

		switch {
		case d.IsDir():
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				return fmt.Errorf("creating %s: %w", destPath, err)
			}
		case d.Type().IsRegular():
			if d.Name() == "pagemap" {
				return nil
			}

			data, err := os.ReadFile(path)
			if err == nil {
				if err := os.WriteFile(destPath, data, 0o644); err != nil {
					return fmt.Errorf("writing file %s: %w", destPath, err)
				}
			}
		case d.Type()&fs.ModeSymlink != 0:
			linkVal, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("readlink %s: %w", path, err)
			}
			if err := os.Symlink(linkVal, destPath); err != nil && !errors.Is(err, fs.ErrExist) {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	for _, c := range p.children {
		if err := dump(pidPathCustom(dir, p.id), c); err != nil {
			return err
		}
	}

	return nil
}

type matchState struct {
	level             int
	forceMatch        bool
	matchFn           func(*process) bool
	matchedDirectly   map[int]bool
	matchedByChildren map[int]bool
}

func (t *Tree) printProcess(p *process, state matchState) {
	state.forceMatch = state.matchDirectly(p) || state.forceMatch

	if !state.matchByChildren(p) && !state.forceMatch {
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

func (s *matchState) matchDirectly(p *process) bool {
	matched, ok := s.matchedDirectly[p.id]
	if !ok {
		matched = s.matchFn(p)
		s.matchedDirectly[p.id] = matched
	}

	return matched
}

func (s *matchState) matchByChildren(p *process) bool {
	matched, ok := s.matchedByChildren[p.id]
	if ok {
		return matched
	}

	for _, c := range p.children {
		if matched = s.matchDirectly(c) || s.matchByChildren(c); matched {
			break
		}
	}

	s.matchedByChildren[p.id] = matched

	return matched
}

const (
	indent = "  "
)
