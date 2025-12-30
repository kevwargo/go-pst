package benchmark

import (
	"encoding/json"
	"os"
	"time"
)

func Register(name string, ts time.Time) {
	dur := time.Since(ts)

	b := benchmarks[name]
	if b == nil {
		b = new(benchmark)
		benchmarks[name] = b
		b.Min = dur
		b.Max = dur
	} else {
		b.Min = min(b.Min, dur)
		b.Max = max(b.Max, dur)
	}

	b.sum += dur
	b.count++
	b.Avg = b.sum / time.Duration(b.count)
}

func Dump() {
	if len(benchmarks) == 0 {
		return
	}

	enc := json.NewEncoder(os.Stderr)
	enc.SetIndent("", "  ")
	enc.Encode(benchmarks)
}

type benchmark struct {
	Min time.Duration
	Max time.Duration
	Avg time.Duration

	sum   time.Duration
	count int
}

var benchmarks = make(map[string]*benchmark)
