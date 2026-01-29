package benchmark

import (
	"encoding/json"
	"os"
	"time"
)

func Record(name string, ts time.Time) {
	dur := duration(time.Since(ts))

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
	b.Avg = b.sum / duration(b.count)
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
	Min duration
	Max duration
	Avg duration

	sum   duration
	count int
}

type duration time.Duration

func (d duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

var benchmarks = make(map[string]*benchmark)
