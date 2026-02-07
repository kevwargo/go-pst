package tree

import (
	"fmt"
	"strconv"
	"strings"
)

type ugid interface {
	id() string
}

type scalarUGID int

func (s scalarUGID) id() string {
	return strconv.Itoa(int(s))
}

type multiUGID struct {
	real       int
	effective  int
	savedSet   int
	filesystem int
}

func (m multiUGID) id() string {
	return fmt.Sprintf("(r:%d e:%d ss:%d fs:%d)", m.real, m.effective, m.savedSet, m.filesystem)
}

func parseUGID(raw string) (_ ugid, err error) {
	parts := strings.Split(raw, "\t")
	if len(parts) != ugidFieldsCount {
		return nil, fmt.Errorf("invalid UGID %q: wrong number of fields, expected %d", raw, ugidFieldsCount)
	}

	values := make([]int, ugidFieldsCount)
	unique := make(map[int]struct{})
	for i, s := range parts {
		values[i], err = strconv.Atoi(s)
		if err != nil {
			return nil, err
		}
		unique[values[i]] = struct{}{}
	}

	if len(unique) == 1 {
		return scalarUGID(values[0]), nil
	}

	return multiUGID{
		real:       values[0],
		effective:  values[1],
		savedSet:   values[2],
		filesystem: values[3],
	}, nil
}

const (
	ugidFieldsCount = 4
)
