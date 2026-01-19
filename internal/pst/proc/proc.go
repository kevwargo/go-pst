package proc

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
)

type Config struct {
	Workdir      bool
	NamespacePID bool
	Threads      bool
	FDs          bool
}

type Process struct {
	ID       int
	ParentID int
	Attrs    Attrs
	Children []*Process // Not actually filled in this package
	Threads  []*Thread
	FDs      []FileDes
	Exit     *ExitStatus
}

type FileDes struct {
	Num  int
	Link string
}

type Attrs struct {
	Name    string
	Args    []string
	Workdir string
	NSPid   []string
}

type Thread struct {
	ID   int
	Name string
	Dead bool
}

type ExitStatus struct {
	Code   int
	Signal int
}

func Load(pid int, cfg *Config) (*Process, error) {
	p := Process{ID: pid}
	if err := p.Reload(cfg); err != nil {
		return nil, err
	}

	return &p, nil
}

func LoadAll(cfg *Config) ([]*Process, error) {
	var ps []*Process

	for pid, err := range intDirEntries(procRoot) {
		if err != nil {
			return nil, err
		}

		p, err := Load(pid, cfg)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}

			return nil, err
		}

		ps = append(ps, p)
	}

	return ps, nil
}

func (p *Process) Reload(cfg *Config) error {
	if err := p.loadAttrs(cfg); err != nil {
		return err
	}

	if err := p.loadThreads(cfg); err != nil {
		return err
	}

	if err := p.loadFDs(cfg); err != nil {
		return err
	}

	return nil
}

func (p *Process) Fork(newPID int) *Process {
	return &Process{
		ID:       newPID,
		ParentID: p.ID,
		Attrs: Attrs{
			Name:    p.Attrs.Name,
			Args:    p.Attrs.Args,
			Workdir: p.Attrs.Workdir,
		},
	}
}

func (a *Attrs) Cmdline() string {
	if !slices.ContainsFunc(a.Args, func(a string) bool {
		return a == "" || strings.ContainsAny(a, " \t")
	}) {
		return strings.Join(a.Args, " ")
	}

	jsonArgs, _ := json.Marshal(a.Args)

	return string(jsonArgs)
}

func (p *Process) loadAttrs(cfg *Config) error {
	cmdline, err := readCmdline(p.ID)
	if err != nil {
		return err
	}

	raw, err := readAttrsMap(p.ID)
	if err != nil {
		return err
	}

	p.ParentID, err = strconv.Atoi(raw["PPid"])
	if err != nil {
		return fmt.Errorf("invalid PPid %q for Pid %d: %w", raw["PPid"], p.ID, err)
	}

	p.Attrs.Args = cmdline
	if n, ok := raw["Name"]; ok {
		p.Attrs.Name = n
	} else if len(cmdline) > 0 {
		p.Attrs.Name = cmdline[0]
	}

	if cfg.Workdir {
		p.Attrs.Workdir, err = os.Readlink(pidPath(p.ID, "cwd"))
		if err != nil {
			p.Attrs.Workdir = fmt.Sprintf("!%s", err.Error())
		}
	}

	if cfg.NamespacePID {
		p.Attrs.NSPid = strings.Split(raw["NSpid"], "\t")
	}

	return nil
}

func (p *Process) loadThreads(cfg *Config) error {
	p.Threads = nil

	if !cfg.Threads {
		return nil
	}

	for tid, err := range intDirEntries(pidPath(p.ID, "task")) {
		if err != nil {
			return err
		}

		if tid == p.ID {
			continue
		}

		if err = p.LoadThread(tid); err != nil {
			return err
		}
	}

	return nil
}

func (p *Process) LoadThread(tid int) error {
	attrs, err := readAttrsMap(tid)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}

	p.Threads = append(p.Threads, &Thread{
		ID:   tid,
		Name: attrs["Name"],
	})

	return nil
}

func (p *Process) loadFDs(cfg *Config) error {
	if !cfg.FDs {
		return nil
	}

	for fd, err := range intDirEntries(pidPath(p.ID, "fd")) {
		if err != nil {
			if errors.Is(err, os.ErrPermission) {
				return nil
			}

			return err
		}

		link, err := os.Readlink(pidPath(p.ID, "fd", strconv.Itoa(fd)))
		if err != nil {
			link = fmt.Sprintf("error:[%s]", err.Error())
		}

		p.FDs = append(p.FDs, FileDes{Num: fd, Link: link})
	}

	slices.SortFunc(p.FDs, func(a, b FileDes) int { return a.Num - b.Num })

	return nil
}
