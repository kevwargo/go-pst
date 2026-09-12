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
	return readStrings(pidPath(pid, "cmdline"))
}

func readEnv(pid int) (map[string]string, error) {
	entries, err := readStrings(pidPath(pid, "environ"))
	if err != nil {
		return nil, err
	}

	envs := make(map[string]string)
	for _, e := range entries {
		parts := strings.SplitN(e, "=", 2)
		name := parts[0]
		var value string
		if len(parts) > 1 {
			value = parts[1]
		}

		envs[name] = value
	}

	return envs, nil
}

func readStrings(filename string) ([]string, error) {
	raw, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	raw = bytes.Trim(raw, "\000")

	if len(raw) == 0 {
		return nil, nil
	}

	parts := bytes.Split(raw, []byte{0})
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		result = append(result, string(p))
	}

	return result, nil
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
