package tree

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func intDirEntries(path string) iter.Seq2[int, error] {
	return func(yield func(int, error) bool) {
		d, err := os.Open(path)
		if err != nil {
			yield(0, fmt.Errorf("open(%s): %w", path, err))
			return
		}
		defer d.Close()

		for {
			entries, err := d.ReadDir(dirBatchSize)
			if errors.Is(err, io.EOF) {
				return
			}

			if err != nil {
				yield(0, err)
				return
			}

			for _, e := range entries {
				val, err := strconv.Atoi(e.Name())
				if err != nil {
					continue
				}

				if !yield(val, nil) {
					return
				}
			}
		}
	}
}

func readCmdline(pid int) ([]string, error) {
	cmdlineRaw, err := os.ReadFile(pidPath(pid, "cmdline"))
	if err != nil {
		return nil, err
	}

	if len(cmdlineRaw) == 0 {
		return nil, nil
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

func pidPath(pid int, parts ...string) string {
	parts = append([]string{procRoot, strconv.Itoa(pid)}, parts...)
	return filepath.Join(parts...)
}

const (
	procRoot     = "/proc"
	dirBatchSize = 100
)
