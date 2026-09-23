package tree

import (
	"fmt"
	"io"
)

type memUsage struct {
	VmRSS   string
	RssAnon string
	RssFile string
	VmSwap  string
}

func (m *memUsage) load(raw map[string]string) {
	*m = memUsage{
		VmRSS:   normalizeSize(raw["VmRSS"]),
		RssAnon: normalizeSize(raw["RssAnon"]),
		RssFile: normalizeSize(raw["RssFile"]),
		VmSwap:  normalizeSize(raw["VmSwap"]),
	}
}

func (m *memUsage) render() string {
	return fmt.Sprintf("Mem:%s anon:%s file:%s swap:%s", m.VmRSS, m.RssAnon, m.RssFile, m.VmSwap)
}

func (m *memUsage) renderTo(w io.Writer) {
	fmt.Fprintf(w, "Mem:%s anon:%s file:%s swap:%s", m.VmRSS, m.RssAnon, m.RssFile, m.VmSwap)
}

func normalizeSize(stringKB string) string {
	const (
		KB = 1
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	var n int
	fmt.Sscanf(stringKB, "%d kB", &n)

	switch {
	case n >= TB:
		return fmt.Sprintf("%.1fT", float64(n)/TB)
	case n >= GB:
		return fmt.Sprintf("%.1fG", float64(n)/GB)
	case n >= MB:
		return fmt.Sprintf("%.1fM", float64(n)/MB)
	default:
		return fmt.Sprintf("%dk", n)
	}
}
