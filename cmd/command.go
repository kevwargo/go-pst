package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
)

func execute(cfg *config, pattern string) error {
	processes, err := collectProcesses(cfg)
	if err != nil {
		return err
	}

	for _, p := range processes {
		if p.matches(cfg, pattern) {
			p.print(cfg, pattern, 0)
		}
	}

	return nil
}

type process struct {
	ID       int        `json:"id"`
	ParentID int        `json:"parentID"`
	Name     string     `json:"name"`
	Args     []string   `json:"args"`
	Workdir  string     `json:"workdir"`
	Threads  []thread   `json:"threads"`
	Children []*process `json:"children"`

	attrs map[string]string
}

type thread struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func collectProcesses(cfg *config) ([]*process, error) {
	d, err := os.Open(procDir)
	if err != nil {
		return nil, fmt.Errorf("open(%s): %w", procDir, err)
	}
	defer d.Close()

	byPid := make(map[int]*process)
	selfPid := os.Getpid()

	err = readPidDir(procDir, func(pid int) error {
		if pid == selfPid {
			return nil
		}

		p, err := makeProcess(pid, cfg)
		if err == nil {
			byPid[pid] = p
		} else if errors.Is(err, os.ErrNotExist) {
			err = nil
		}

		return err
	})
	if err != nil {
		return nil, err
	}

	var processes []*process

	for _, p := range byPid {
		if p.ParentID < 1 {
			processes = append(processes, p)
		} else {
			if parent := byPid[p.ParentID]; parent != nil {
				parent.Children = append(parent.Children, p)
			}
		}
	}

	return processes, nil
}

func makeProcess(pid int, cfg *config) (*process, error) {
	cmdline, err := readCmdline(pid)
	if err != nil {
		return nil, err
	}

	attrs, err := readAttrs(fmt.Sprintf("%s/%d/status", procDir, pid))
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
		ID:       pid,
		ParentID: ppid,
		Name:     name,

		attrs: attrs,
	}

	if len(cmdline) > 1 {
		p.Args = cmdline[1:]
	}

	if cfg.showWorkdir {
		p.Workdir, err = os.Readlink(fmt.Sprintf("%s/%d/cwd", procDir, pid))
		if err != nil {
			p.Workdir = fmt.Sprintf("!%s", err.Error())
		}
	}

	if cfg.showThreads {
		p.Threads, err = readThreads(pid)
		if err != nil {
			return nil, err
		}
	}

	return &p, nil
}

func readThreads(pid int) ([]thread, error) {
	var (
		threads []thread
		taskDir = fmt.Sprintf("%s/%d/task", procDir, pid)
	)

	err := readPidDir(taskDir, func(tid int) error {
		if tid == pid {
			return nil
		}

		attrs, err := readAttrs(fmt.Sprintf("%s/%d/status", taskDir, tid))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		} else if err != nil {
			return err
		}

		threads = append(threads, thread{
			ID:   tid,
			Name: attrs["Name"],
		})

		return nil
	})

	return threads, err
}

func readPidDir(dir string, fn func(int) error) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open(%s): %w", dir, err)
	}

	for {
		entries, err := d.ReadDir(dirBatchSize)
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return err
		}

		for _, e := range entries {
			pid, err := strconv.Atoi(e.Name())
			if err != nil {
				continue
			}

			if err = fn(pid); err != nil {
				return err
			}
		}
	}
}

func readCmdline(pid int) ([]string, error) {
	cmdlineRaw, err := os.ReadFile(fmt.Sprintf("%s/%d/cmdline", procDir, pid))
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

func (p *process) render(cfg *config) string {
	pstr := fmt.Sprintf("[%d]%s%s %s", p.ID, p.renderGUIDs(cfg), p.renderWorkdir(cfg), p.renderCmdline())
	if cfg.truncate > 0 && len(pstr) > cfg.truncate {
		pstr = pstr[:cfg.truncate]
	}

	return pstr
}

func (p *process) print(cfg *config, pattern string, level int) {
	indent := strings.Repeat("  ", level)
	fmt.Printf("%s%s\n", indent, p.render(cfg))

	for _, t := range p.Threads {
		fmt.Printf("%s  [%d]{%s}\n", indent, t.ID, t.Name)
	}

	for _, c := range p.Children {
		if c.matches(cfg, pattern) {
			c.print(cfg, pattern, level+1)
		}
	}
}

func (p *process) matches(cfg *config, pattern string) bool {
	for _, c := range p.Children {
		if c.matches(cfg, pattern) {
			return true
		}
	}

	if cfg.fullMatch {
		return strings.Contains(p.renderCmdline(), pattern)
	}

	args := []string{strconv.Itoa(p.ID), p.Name}
	args = append(args, p.Args...)

	return slices.ContainsFunc(args, func(a string) bool {
		return strings.Contains(a, pattern)
	})
}

func (p *process) renderGUIDs(cfg *config) string {
	if !cfg.showUID && !cfg.showGID {
		return ""
	}

	return ""
}

func (p *process) renderWorkdir(cfg *config) string {
	if !cfg.showWorkdir {
		return ""
	}

	return fmt.Sprintf(" (%s)", p.Workdir)
}

func (p *process) renderCmdline() string {
	if len(p.Args) == 0 {
		return p.Name
	}

	var buf strings.Builder

	buf.WriteString(p.Name)
	buf.WriteByte(' ')

	var containsSpace bool
	for _, a := range p.Args {
		if strings.ContainsAny(a, " \t") {
			containsSpace = true
			break
		}
	}

	if containsSpace {
		json.NewEncoder(&buf).Encode(p.Args)
	} else {
		buf.WriteString(strings.Join(p.Args, " "))
	}

	return buf.String()
}

const (
	procDir      = "/proc"
	dirBatchSize = 100
)
