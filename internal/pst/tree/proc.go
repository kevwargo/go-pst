package tree

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
)

type ProcConfig struct {
	Workdir      bool
	UGID         bool
	NamespacePID bool
	Threads      bool
	FDs          bool
}

type process struct {
	id       int
	parentID int
	attrs    attrs
	threads  []*thread
	fds      []fileDes
	exit     *exitStatus
	children []*process
}

type fileDes struct {
	num  int
	link string
}

type attrs struct {
	name    string
	args    []string
	workdir string
	uid     ugid
	gid     ugid
	nsPid   []string
}

type thread struct {
	id   int
	name string
	dead bool
}

type exitStatus struct {
	code   int
	signal int
}

func loadProc(pid int, cfg *ProcConfig) (*process, error) {
	p := process{id: pid}
	if err := p.reload(cfg); err != nil {
		return nil, err
	}

	return &p, nil
}

func (p *process) reload(cfg *ProcConfig) error {
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

func (p *process) fork(newPID int) *process {
	child := &process{
		id:       newPID,
		parentID: p.id,
		attrs: attrs{
			name:    p.attrs.name,
			args:    p.attrs.args,
			workdir: p.attrs.workdir,
		},
	}

	p.children = append(p.children, child)

	return child
}

func (a *attrs) cmdline() string {
	if a.args == nil {
		return fmt.Sprintf("*%s*", a.name)
	}

	if !slices.ContainsFunc(a.args, func(a string) bool {
		return a == "" || strings.ContainsAny(a, " \t\n")
	}) {
		return strings.Join(a.args, " ")
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(a.args)

	data := buf.Bytes()
	for last := buf.Len() - 1; data[last] == '\n'; last-- {
		data = data[:last]
	}

	return string(data)
}

func (p *process) loadAttrs(cfg *ProcConfig) error {
	cmdline, err := readCmdline(p.id)
	if err != nil {
		return err
	}

	raw, err := readAttrsMap(p.id)
	if err != nil {
		return err
	}

	p.parentID, err = strconv.Atoi(raw["PPid"])
	if err != nil {
		return fmt.Errorf("invalid PPid %q for Pid %d: %w", raw["PPid"], p.id, err)
	}

	p.attrs.args = cmdline
	if n, ok := raw["Name"]; ok {
		p.attrs.name = n
	} else if len(cmdline) > 0 {
		p.attrs.name = cmdline[0]
	}

	if cfg.Workdir {
		p.attrs.workdir, err = os.Readlink(pidPath(p.id, "cwd"))
		if err != nil {
			p.attrs.workdir = fmt.Sprintf("!%s", err.Error())
		}
	}

	if cfg.UGID {
		p.attrs.uid, err = parseUGID(raw["Uid"])
		if err != nil {
			return err
		}
		p.attrs.gid, err = parseUGID(raw["Gid"])
		if err != nil {
			return err
		}
	}

	if cfg.NamespacePID {
		p.attrs.nsPid = strings.Split(raw["NSpid"], "\t")
	}

	return nil
}

func (p *process) loadThreads(cfg *ProcConfig) error {
	p.threads = nil

	if !cfg.Threads {
		return nil
	}

	for tid, err := range intDirEntries(pidPath(p.id, "task")) {
		if err != nil {
			return err
		}

		if tid == p.id {
			continue
		}

		if err = p.loadThread(tid); err != nil {
			return err
		}
	}

	return nil
}

func (p *process) loadThread(tid int) error {
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
}

func (p *process) loadFDs(cfg *ProcConfig) error {
	if !cfg.FDs {
		return nil
	}

	p.fds = nil

	for fd, err := range intDirEntries(pidPath(p.id, "fd")) {
		if err != nil {
			if errors.Is(err, os.ErrPermission) {
				return nil
			}

			return err
		}

		link, err := os.Readlink(pidPath(p.id, "fd", strconv.Itoa(fd)))
		if err != nil {
			link = fmt.Sprintf("error:[%s]", err.Error())
		}

		p.fds = append(p.fds, fileDes{num: fd, link: link})
	}

	slices.SortFunc(p.fds, func(a, b fileDes) int { return a.num - b.num })

	return nil
}
