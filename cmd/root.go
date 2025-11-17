package cmd

import "github.com/spf13/cobra"

func Execute() error {
	return rootCmd().Execute()
}

type config struct {
	fullMatch         bool
	executableMatch   bool
	showThreads       bool
	showWorkdir       bool
	showUID           bool
	showGID           bool
	showBasicFDs      bool
	showProcessGroups bool
	truncate          int
}

func rootCmd() *cobra.Command {
	cfg := new(config)

	cmd := &cobra.Command{
		Use:           "pst [flags] PATTERN",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return execute(cfg, args[0])
		},
	}

	f := cmd.Flags()
	f.BoolVarP(&cfg.fullMatch, "full-match", "f", false, "")
	f.BoolVarP(&cfg.executableMatch, "executable-match", "X", false, "")
	f.BoolVarP(&cfg.showThreads, "show-threads", "T", false, "")
	f.BoolVarP(&cfg.showWorkdir, "show-workdir", "w", false, "")
	f.BoolVarP(&cfg.showUID, "show-uid", "u", false, "")
	f.BoolVarP(&cfg.showGID, "show-gid", "g", false, "")
	f.BoolVarP(&cfg.showBasicFDs, "show-basic-fds", "F", false, "")
	f.BoolVarP(&cfg.showProcessGroups, "show-process-groups", "G", false, "")

	return cmd
}
