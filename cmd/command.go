package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func execute(cfg config, pattern string) error {
	processes, err := collectProcesses()
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(processes)
}

type process struct {
	ID       int        `json:"id"`
	ParentID int        `json:"parentID"`
	Name     string     `json:"name"`
	Cmdline  []string   `json:"cmdline"`
	Children []*process `json:"children"`
}

func collectProcesses() ([]*process, error) {
	d, err := os.Open(procDir)
	if err != nil {
		return nil, fmt.Errorf("open(%s): %w", procDir, err)
	}
	defer d.Close()

	byPid := make(map[int]*process)
	selfPid := os.Getpid()

	for {
		entries, err := d.ReadDir(dirBatchSize)
		if errors.Is(err, io.EOF) {
			break
		}

		for _, e := range entries {
			pid, err := strconv.Atoi(e.Name())
			if err != nil {
				continue
			}

			if pid == selfPid {
				continue
			}

			p, err := makeProcess(pid)
			if err != nil {
				return nil, err
			}

			byPid[pid] = p
		}
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

func makeProcess(pid int) (*process, error) {
	cmdline, err := readCmdline(pid)
	if err != nil {
		return nil, err
	}

	attrs, err := readAttrs(pid)
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

	return &process{
		ID:       pid,
		ParentID: ppid,
		Name:     name,
		Cmdline:  cmdline,
	}, nil
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

func readAttrs(pid int) (map[string]string, error) {
	f, err := os.Open(fmt.Sprintf("%s/%d/status", procDir, pid))
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

const (
	procDir      = "/proc"
	dirBatchSize = 100
)
