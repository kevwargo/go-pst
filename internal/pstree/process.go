package pstree

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type process struct {
	id       int
	parentID int
	attrs    procAttrs
	children []*process
	threads  []*thread
	fds      map[int]string
}

type procAttrs struct {
	name    string
	args    []string
	workdir string
	nsPid   []int

	raw map[string]string
}

type thread struct {
	id   int
	name string
}

func loadProcess(pid int, cfg *Config) (*process, error) {
	p := process{id: pid}

	if err := p.loadAttrs(cfg); err != nil {
		return nil, err
	}

	if err := p.loadThreads(cfg); err != nil {
		return nil, err
	}

	if err := p.loadFDs(cfg); err != nil {
		return nil, err
	}

	return &p, nil
}

func (p *process) loadAttrs(cfg *Config) error {
	cmdline, err := readCmdline(p.id)
	if err != nil {
		return err
	}

	p.attrs.raw, err = readAttrsMap(p.id)
	if err != nil {
		return err
	}

	p.parentID, err = strconv.Atoi(p.attrs.raw["PPid"])
	if err != nil {
		return fmt.Errorf("invalid PPid %q for Pid %d: %w", p.id, err)
	}

	if len(cmdline) > 0 {
		p.attrs.name = cmdline[0]
		p.attrs.args = cmdline[1:]
	} else if n, ok := p.attrs.raw["Name"]; ok {
		p.attrs.name = fmt.Sprintf("*%s*", n)
	}

	if cfg.ShowWorkdir {
		p.attrs.workdir, err = os.Readlink(pidPath(p.id, "cwd"))
		if err != nil {
			p.attrs.workdir = fmt.Sprintf("!%s", err.Error())
		}
	}

	if cfg.ShowNamespacePID {
		strPids := strings.Split(p.attrs.raw["NSpid"], "\t")
		if len(strPids) > 1 {
			p.attrs.nsPid = make([]int, len(strPids))
			for i, sp := range strPids {
				p.attrs.nsPid[i], err = strconv.Atoi(sp)
				if err != nil {
					return fmt.Errorf("invalid entry %q in %d/status/NSpid: %w", sp, p.id, err)
				}
			}
		}
	}

	return nil
}

func (p *process) loadThreads(cfg *Config) error {
	if !cfg.ShowThreads {
		return nil
	}

	err := iterIntDirEntries(pidPath(p.id, "task"), func(tid int) error {
		if !cfg.ShowMainThread && tid == p.id {
			return nil
		}

		attrs, err := readAttrsMap(tid)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		} else if err != nil {
			return err
		}

		p.threads = append(p.threads, &thread{
			id:   tid,
			name: attrs["Name"],
		})

		return nil
	})

	return err
}

func (p *process) loadFDs(cfg *Config) error {
	if !cfg.InspectAllFDs {
		return nil
	}

	return iterIntDirEntries(pidPath(p.id, "fd"), func(fd int) error {
		link, err := os.Readlink(pidPath(p.id, "fd", strconv.Itoa(fd)))
		if err != nil {
			link = fmt.Sprintf("error:[%s]", err.Error())
		}

		p.fds[fd] = link

		return nil
	})
}

func (p *process) format(cfg *Config) string {
	pstr := fmt.Sprintf("%s%s %s",
		p.formatPid(),
		p.attrs.formatWorkdir(),
		p.attrs.formatCmdline(),
	)

	if cfg.Truncate > 0 && len(pstr) > cfg.Truncate {
		pstr = pstr[:cfg.Truncate]
	}

	return pstr
}

func (p *process) formatPid() string {
	if p.attrs.nsPid == nil {
		return fmt.Sprintf("[%d]", p.id)
	}

	return fmt.Sprint(p.attrs.nsPid)
}

func (a *procAttrs) formatWorkdir() string {
	if a.workdir == "" {
		return ""
	}

	return fmt.Sprintf(" (%s)", a.workdir)
}

func (a *procAttrs) cmdline() []string {
	return append([]string{a.name}, a.args...)
}

func (a *procAttrs) formatCmdline() string {
	args := a.cmdline()

	if !slices.ContainsFunc(args, func(a string) bool {
		return a == "" || strings.ContainsAny(a, " \t")
	}) {
		return strings.Join(args, " ")
	}

	jsonArgs, _ := json.Marshal(args)

	return string(jsonArgs)
}

func (t *thread) format() string {
	return fmt.Sprintf("{%d} %s", t.id, t.name)
}

func pidPath(pid int, parts ...string) string {
	return pidPathCustom(procDir, pid, parts...)
}

func pidPathCustom(baseDir string, pid int, parts ...string) string {
	parts = append([]string{baseDir, strconv.Itoa(pid)}, parts...)
	return filepath.Join(parts...)
}

func iterIntDirEntries(path string, fn func(int) error) error {
	d, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open(%s): %w", path, err)
	}
	defer d.Close()

	for {
		entries, err := d.ReadDir(dirBatchSize)
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return err
		}

		for _, e := range entries {
			val, err := strconv.Atoi(e.Name())
			if err != nil {
				continue
			}

			if err = fn(val); err != nil {
				return err
			}
		}
	}
}

func readCmdline(pid int) ([]string, error) {
	cmdlineRaw, err := os.ReadFile(pidPath(pid, "cmdline"))
	if err != nil {
		return nil, err
	}

	cmdlineBytes := bytes.Split(cmdlineRaw, []byte{0})
	cmdline := make([]string, 0, len(cmdlineBytes)-1)
	for i := range len(cmdlineBytes) - 1 {
		cmdline = append(cmdline, string(cmdlineBytes[i]))
	}

	return cmdline, nil
}

func readAttrsMap(pid int) (map[string]string, error) {
	f, err := os.Open(pidPath(pid, "status"))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	attrs := make(map[string]string)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}

		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}

		attrs[parts[0]] = strings.Trim(parts[1], " \t\n")
	}

	return attrs, nil
}

const (
	procDir      = "/proc"
	dirBatchSize = 100
)
