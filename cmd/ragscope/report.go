package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/piovani/RAGscope/internal/report"
)

func newReportCmd() *cobra.Command {
	var resultsDir, baselineName string

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Compare the saved results of all experiments in a Markdown table",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			all, err := report.Load(resultsDir)
			if err != nil {
				return err
			}
			reports := report.Latest(all)

			// "baseline" is only a default: if it is missing we just skip the
			// comparison. A name the user typed must exist.
			baseline := report.Find(reports, baselineName)
			if baseline == nil && cmd.Flags().Changed("baseline") {
				return fmt.Errorf("no results found for the baseline experiment %q", baselineName)
			}

			report.Markdown(cmd.OutOrStdout(), reports, baseline)
			return nil
		},
	}

	cmd.Flags().StringVar(&resultsDir, "results", "experiments/results", "folder with the results saved by eval")
	cmd.Flags().StringVar(&baselineName, "baseline", "baseline", "experiment the others are compared to")
	return cmd
}
