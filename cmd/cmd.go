package cmd

import (
	"fmt"

	"github.com/kevwargo/go-pst/internal/benchmark"
	"github.com/kevwargo/go-pst/internal/pst/tree"
	"github.com/kevwargo/go-pst/internal/pst/tui"
	"github.com/spf13/cobra"
)

func Execute() error {
	var cfg config

	cmd := &cobra.Command{
		Use:           "pst",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, args []string) error {
			return execute(&cfg, args)
		},
	}

	bindFlags(cmd.Flags(), &cfg)

	return cmd.Execute()
}

type config struct {
	tree             tree.Config
	tui              tui.Config
	fitTerm          bool
	interactive      bool
	dumpProcSnapshot string
	showBenchmarks   bool
}

func execute(cfg *config, args []string) error {
	if cfg.showBenchmarks {
		defer benchmark.Dump()
	}

	if cfg.interactive {
		cfg.tree.FitTermHeight = true
		cfg.tree.FitTermWidth = true
	} else if cfg.fitTerm {
		cfg.tree.FitTermWidth = true
	}

	pst, err := tree.Build(&cfg.tree)
	if err != nil {
		return err
	}

	if cfg.dumpProcSnapshot != "" {
		return dumpProcSnapshot(cfg.dumpProcSnapshot, pst)
	}

	if cfg.tree.PCfg.FDs {
		return inspectAllFDs(pst)
	}

	if len(args) != 1 {
		return fmt.Errorf("invalid number of positional arguments: %d (must be 1)", len(args))
	}

	pst.Filter(args[0])

	if cfg.interactive {
		return tui.Run(&cfg.tui, pst)
	}

	fmt.Println(pst.View())

	return nil
}

func dumpProcSnapshot(path string, pst *tree.Tree) error {
	return nil
}

func inspectAllFDs(pst *tree.Tree) error {
	return nil
}
