package tree

import (
	"errors"
	"os"
	"slices"
)

func loadPMap(cfg *ProcConfig) (map[int]*process, error) {
	m := make(map[int]*process)

	for pid, err := range intDirEntries(procRoot) {
		if err != nil {
			return nil, err
		}

		p, err := loadProc(pid, cfg)
		switch {
		case err == nil:
			m[p.id] = p
		case !errors.Is(err, os.ErrNotExist):
			return nil, err
		}
	}

	removeSelf(m)

	return m, nil
}

func removeSelf(m map[int]*process) {
	p := m[os.Getpid()]
	if p == nil {
		return
	}

	delete(m, p.id)

	for parent := m[p.parentID]; isSudoAncestor(parent, p); parent = m[parent.parentID] {
		delete(m, parent.id)
	}
}

func isSudoAncestor(ancestor, descendant *process) bool {
	if ancestor == nil {
		return false
	}

	return slices.Equal(
		ancestor.attrs.args,
		append([]string{"sudo"}, descendant.attrs.args...),
	)
}
