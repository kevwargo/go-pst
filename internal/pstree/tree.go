package pstree

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
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
		return t.inspectFDs()
	}

	t.filter(pattern)

	if t.cfg.Interactive {
		return runTUI(t)
	}

	t.dump(os.Stdout)

	return nil
}

func (t *Tree) filter(pattern string) {
	var matchFn func(*process) bool
	if t.cfg.FullMatch {
		matchFn = func(p *process) bool {
			return strings.Contains(p.formatCmdline(), pattern)
		}
	} else {
		pid, err := strconv.Atoi(pattern)
		if err != nil {
			pid = -1
		}

		matchFn = func(p *process) bool {
			if p.id == pid {
				return true
			}

			return slices.ContainsFunc(append([]string{p.name}, p.args...), func(a string) bool {
				return strings.Contains(a, pattern)
			})
		}
	}

	t.topLevel = slices.DeleteFunc(t.topLevel, func(p *process) bool {
		return !t.filterProcess(p, matchFn)
	})
}

func (t *Tree) filterProcess(p *process, match func(*process) bool) bool {
	if match(p) {
		return true
	}

	p.children = slices.DeleteFunc(p.children, func(c *process) bool {
		return !t.filterProcess(c, match)
	})

	if len(p.children) > 0 {
		return true
	}

	delete(t.pMap, p.id)

	return false
}

func (t *Tree) dump(w io.Writer) {
	for _, p := range t.topLevel {
		t.dumpProcess(p, w, 0)
	}
}

func (t *Tree) dumpProcess(p *process, w io.Writer, level int) {
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
