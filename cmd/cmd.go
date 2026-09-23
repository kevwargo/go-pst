package cmd

import (
	"fmt"

	"charm.land/lipgloss/v2"
	"github.com/kevwargo/go-pst/internal/benchmark"
	"github.com/kevwargo/go-pst/internal/logging"
	"github.com/kevwargo/go-pst/internal/pst/tree"
	"github.com/kevwargo/go-pst/internal/pst/tui"
	"github.com/spf13/cobra"
)

func Execute() error {
	var cfg config

	cmd := &cobra.Command{
		Use:           "pst [flags] PATTERN",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, args []string) error {
			if cfg.interactive {
				revertLogging := logging.Redirect()
				defer revertLogging()
			}

			return execute(&cfg, args)
		},
	}

	fs := cmd.Flags()
	fs.BoolVarP(&cfg.tree.PCfg.Workdir, "workdir", "w", false, "")
	fs.BoolVarP(&cfg.tree.PCfg.UGID, "uid-gid", "u", false, "")
	fs.BoolVarP(&cfg.tree.PCfg.NamespacePID, "namespace-pid", "N", false, "")
	fs.BoolVarP(&cfg.tree.PCfg.Threads, "threads", "T", false, "")
	fs.BoolVarP(&cfg.tree.PCfg.FDs, "file-descriptors", "F", false, "")
	fs.BoolVarP(&cfg.tree.PCfg.PathEnv, "path-env", "P", false, "")
	fs.BoolVarP(&cfg.tree.PCfg.MemoryUsage, "memory-usage", "M", false, "")
	fs.BoolVarP(&cfg.tree.ShowDead, "show-dead", "D", false, "")
	fs.BoolVarP(&cfg.tree.IgnoreCase, "ignore-case", "I", false, "")
	fs.BoolVar(
		&cfg.tree.PCfg.EphemeralStats,
		"ephemeral",
		false,
		"Show threads/fds even for processes not matched directly",
	)

	fs.BoolVarP(&cfg.interactive, "interactive", "i", false, "")
	fs.BoolVarP(&cfg.tui.Fullscreen, "fullscreen", "A", false, "")
	fs.BoolVar(&cfg.tui.Debug, "debug", false, "")

	// TODO: use different variable maybe
	fs.BoolVar(&cfg.inspectAllFDs, "inspect-all-fds", false, "")
	fs.StringVar(&cfg.dumpProcSnapshot, "dump-process-snapshot", "", "")

	fs.BoolVar(&cfg.showBenchmarks, "benchmarks", false, "")

	fs.StringSliceVar(
		&cfg.tree.PCfg.DebugCmdline,
		"debug-cmdline",
		nil,
		"Log debug information about processes matching this cmdline pattern (as comma-separated list)",
	)

	return cmd.Execute()
}

type config struct {
	tree             tree.Config
	tui              tui.Config
	interactive      bool
	dumpProcSnapshot string
	inspectAllFDs    bool
	showBenchmarks   bool
}

func execute(cfg *config, args []string) error {
	if cfg.showBenchmarks {
		benchmark.Enable()

		defer benchmark.Dump()
	}

	if cfg.interactive {
		cfg.tree.FitTermHeight = true
	}
	if cfg.inspectAllFDs {
		cfg.tree.PCfg.FDs = true
	}

	pst, err := tree.Build(&cfg.tree)
	if err != nil {
		return err
	}

	if cfg.dumpProcSnapshot != "" {
		return dumpProcSnapshot(cfg.dumpProcSnapshot, pst)
	}

	if cfg.inspectAllFDs {
		return inspectAllFDs(pst)
	}

	if len(args) != 1 {
		return fmt.Errorf("invalid number of positional arguments: %d (must be 1)", len(args))
	}

	pst.Filter(args[0])

	if cfg.interactive {
		err = tui.Run(&cfg.tui, pst)
	} else {
		if view := pst.View(); view != "" {
			lipgloss.Println(view)
		}
	}

	return err
}

func dumpProcSnapshot(path string, pst *tree.Tree) error {
	// TODO: implement
	return nil
}

func inspectAllFDs(pst *tree.Tree) error {
	// TODO: implement
	return nil
}
