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
	name     string
	args     []string
	workdir  string
	threads  []thread
	fds      map[int]string
	children []*process

	attrs map[string]string
}

type thread struct {
	id   int
	name string
}

func readProcess(pid int, cfg *Config) (*process, error) {
	cmdline, err := readCmdline(pid)
	if err != nil {
		return nil, err
	}

	attrs, err := readAttrs(pidPath(pid, "status"))
	if err != nil {
		return nil, err
	}

	var name string
	if len(cmdline) > 0 {
		name = cmdline[0]
	} else if n, ok := attrs["Name"]; ok {
		name = fmt.Sprintf("*%s*", n)
	}

	ppid, err := strconv.Atoi(attrs["PPid"])
	if err != nil {
		return nil, fmt.Errorf("invalid PPid for Pid %d: %w", pid, err)
	}

	p := process{
		id:       pid,
		parentID: ppid,
		name:     name,
		fds:      make(map[int]string),

		attrs: attrs,
	}

	if len(cmdline) > 1 {
		p.args = cmdline[1:]
	}

	if cfg.ShowWorkdir {
		p.workdir, err = os.Readlink(pidPath(pid, "cwd"))
		if err != nil {
			p.workdir = fmt.Sprintf("!%s", err.Error())
		}
	}

	if cfg.ShowThreads {
		p.threads, err = readThreads(pid, cfg)
		if err != nil {
			return nil, err
		}
	}

	if cfg.InspectAllFDs {
		if err := iterIntDirEntries(pidPath(pid, "fd"), func(fd int) error {
			link, err := os.Readlink(pidPath(pid, "fd", strconv.Itoa(fd)))
			if err != nil {
				link = fmt.Sprintf("error:[%s]", err.Error())
			}

			p.fds[fd] = link

			return nil
		}); err != nil {
			return nil, err
		}
	}

	return &p, nil
}

func readThreads(pid int, cfg *Config) ([]thread, error) {
	var threads []thread

	err := iterIntDirEntries(pidPath(pid, "task"), func(tid int) error {
		if !cfg.ShowMainThread && tid == pid {
			return nil
		}

		attrs, err := readAttrs(pidPath(tid, "status"))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		} else if err != nil {
			return err
		}

		threads = append(threads, thread{
			id:   tid,
			name: attrs["Name"],
		})

		return nil
	})

	return threads, err
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

func readAttrs(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

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

		attrs[parts[0]] = strings.TrimLeft(parts[1], " \t")
	}

	return attrs, nil
}

func (p *process) format(cfg *Config) string {
	pstr := fmt.Sprintf("[%s]%s%s %s",
		p.formatPid(cfg),
		p.formatGUIDs(cfg),
		p.formatWorkdir(cfg),
		p.formatCmdline(),
	)

	if cfg.Truncate > 0 && len(pstr) > cfg.Truncate {
		pstr = pstr[:cfg.Truncate]
	}

	return pstr
}

func (p *process) formatPid(cfg *Config) string {
	if cfg.ShowNamespacePID {
		return strings.ReplaceAll(p.attrs["NSpid"], "\t", " ")
	}

	return strconv.Itoa(p.id)
}

func (p *process) formatGUIDs(cfg *Config) string {
	if !cfg.ShowUID && !cfg.ShowGID {
		return ""
	}

	return ""
}

func (p *process) formatWorkdir(cfg *Config) string {
	if !cfg.ShowWorkdir {
		return ""
	}

	return fmt.Sprintf(" (%s)", p.workdir)
}

func (p *process) formatCmdline() string {
	if len(p.args) == 0 {
		return p.name
	}

	args := append([]string{p.name}, p.args...)

	if !slices.ContainsFunc(args, func(a string) bool {
		return a == "" || strings.ContainsAny(a, " \t")
	}) {
		return strings.Join(args, " ")
	}

	jsonArgs, _ := json.Marshal(args)

	return string(jsonArgs)
}

func (t thread) format() string {
	return fmt.Sprintf("{%d} %s", t.id, t.name)
}

const (
	procDir      = "/proc"
	dirBatchSize = 100
)
