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
	f.BoolVarP(&cfg.ShowUID, "show-uid", "u", false, "")
	f.BoolVarP(&cfg.ShowGID, "show-gid", "g", false, "")
	f.BoolVarP(&cfg.ShowBasicFDs, "show-basic-fds", "F", false, "")
	f.BoolVarP(&cfg.ShowProcessGroups, "show-process-groups", "G", false, "")
	f.IntVarP(&cfg.Truncate, "truncate", "t", 0, "Truncate lines longer than the passed value")

	f.BoolVar(&cfg.Trace, "enable-trace", false, "Print some debug and tracing information to stderr")

	return cmd.Execute()
}

func execute(cfg *pstree.Config, pattern string) error {
	tree, err := pstree.Build(cfg)
	if err != nil {
		return err
	}

	tree.Print(pattern)

	return nil
}
