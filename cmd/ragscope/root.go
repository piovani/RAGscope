package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/piovani/RAGscope/internal/config"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "ragscope",
		Short:         "Benchmark RAG configurations with real metrics",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newIngestCmd(), newAskCmd(), newEvalCmd(), newEvalAnswersCmd(), newGradeSheetCmd(), newCalibrateCmd(), newGoldenCmd(), newConvertCmd(), newServeCmd(), newReportCmd(), newValidateCmd())
	return root
}

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <experiment.yaml>...",
		Short: "Validate experiment config files",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, path := range args {
				exp, err := config.Load(path)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "ok  %s (%s)\n", path, exp.Name)
			}
			return nil
		},
	}
}
