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
	var cfg config

	cmd := &cobra.Command{
		Use:           "pst [flags] PATTERN",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return execute(cfg, args[0])
		},
	}

	fs := cmd.Flags()
	fs.BoolVarP(&cfg.fullMatch, "full-match", "f", false, "")
	fs.BoolVarP(&cfg.executableMatch, "executable-match", "X", false, "")
	fs.BoolVarP(&cfg.showThreads, "show-threads", "T", false, "")
	fs.BoolVarP(&cfg.showWorkdir, "show-workdir", "w", false, "")
	fs.BoolVarP(&cfg.showUID, "show-uid", "u", false, "")
	fs.BoolVarP(&cfg.showGID, "show-gid", "g", false, "")
	fs.BoolVarP(&cfg.showBasicFDs, "show-basic-fds", "F", false, "")
	fs.BoolVarP(&cfg.showProcessGroups, "show-process-groups", "G", false, "")

	return cmd
}
