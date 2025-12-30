package cmd

import (
	"github.com/spf13/pflag"
)

func bindFlags(fs *pflag.FlagSet, cfg *config) {
	fs.BoolVarP(&cfg.tree.PCfg.Workdir, "workdir", "w", false, "")
	fs.BoolVarP(&cfg.tree.PCfg.NamespacePID, "namespace-pid", "N", false, "")
	fs.BoolVarP(&cfg.tree.PCfg.Threads, "threads", "T", false, "")
	fs.BoolVarP(&cfg.tree.ShowDead, "show-dead", "D", false, "")
	fs.BoolVarP(&cfg.tree.FullMatch, "full-match", "f", false, "")

	fs.BoolVarP(&cfg.interactive, "interactive", "i", false, "")
	fs.BoolVarP(&cfg.tui.Fullscreen, "fullscreen", "A", false, "")
	fs.BoolVarP(&cfg.fitTerm, "fit-terminal-width", "t", false, "")

	// TODO: use different variable maybe
	fs.BoolVar(&cfg.tree.PCfg.FDs, "inspect-all-fds", false, "")
	fs.StringVar(&cfg.dumpProcSnapshot, "dump-process-snapshot", "", "")

	fs.BoolVar(&cfg.showBenchmarks, "benchmarks", false, "")
}
