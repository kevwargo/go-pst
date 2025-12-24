package pst

import (
	"github.com/kevwargo/go-pst/internal/pstree"
	"github.com/spf13/cobra"
)

func Execute() error {
	var cfg pstree.Config

	cmd := &cobra.Command{
		Use:           "pst [flags] PATTERN",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return execute(&cfg, args[0])
		},
	}

	f := cmd.Flags()

	f.BoolVarP(&cfg.FullMatch, "full-match", "f", false, "")
	f.BoolVarP(&cfg.ShowThreads, "show-threads", "T", false, "")
	f.BoolVar(&cfg.ShowMainThread, "show-main-thread", false, "")
	f.BoolVarP(&cfg.ShowWorkdir, "show-workdir", "w", false, "")

	// TODO
	f.BoolVarP(&cfg.ShowUID, "show-uid", "u", false, "")
	f.BoolVarP(&cfg.ShowGID, "show-gid", "g", false, "")
	f.BoolVarP(&cfg.ShowBasicFDs, "show-basic-fds", "F", false, "")
	f.BoolVarP(&cfg.ShowProcessGroups, "show-process-groups", "G", false, "")

	f.BoolVarP(&cfg.ShowNamespacePID, "show-namespace-pid", "N", false, "")
	f.IntVarP(&cfg.Truncate, "truncate", "t", 0, "Truncate lines longer than the passed value")

	f.BoolVarP(&cfg.Interactive, "interactive", "i", false, "Run interactive TUI")
	f.BoolVarP(&cfg.ShowDead, "show-dead", "D", false, "Don't hide exited processes (applies only in TUI)")
	f.BoolVarP(&cfg.Fullscreen, "fullscreen", "A", false, "Start TUI in fullscreen")

	f.BoolVar(&cfg.InspectAllFDs, "inspect-all-fds", false, "Dump info about all open file descriptors across all processes")
	f.StringVar(&cfg.DumpProcessSnapshot, "dump-process-snapshot", "", "Store the current state of the process specified by PATTERN (must be exact pid) and all its ancestors in a directory")

	return cmd.Execute()
}

func execute(cfg *pstree.Config, pattern string) error {
	tree, err := pstree.Build(cfg)
	if err != nil {
		return err
	}

	return tree.Run(pattern)
}
