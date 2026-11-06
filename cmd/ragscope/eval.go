package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/piovani/RAGscope/internal/config"
	"github.com/piovani/RAGscope/internal/eval"
	"github.com/piovani/RAGscope/internal/ingest"
)

func newEvalCmd() *cobra.Command {
	var cfgPath, goldenPath, resultsDir string

	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Score the retrieval of an experiment against the golden set",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			exp, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			questions, err := eval.LoadQuestions(goldenPath)
			if err != nil {
				return err
			}

			db, err := openStore(cmd.Context())
			if err != nil {
				return err
			}
			defer db.Close()

			retriever, err := newRetriever(cmd.Context(), exp, db)
			if err != nil {
				return err
			}

			report, err := eval.RunRetrieval(cmd.Context(), retriever, questions, exp.FinalK())
			if err != nil {
				return err
			}
			report.Experiment = exp.Name
			report.GoldenSet = eval.Fingerprint(questions)

			out := cmd.OutOrStdout()
			report.Print(out)

			path, err := report.Save(resultsDir)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "\nsaved: %s\n", path)
			return nil
		},
	}

	cmd.Flags().StringVarP(&cfgPath, "config", "c", "", "experiment config file")
	cmd.Flags().StringVar(&goldenPath, "golden", "data/golden/questions.yaml", "golden set file")
	cmd.Flags().StringVar(&resultsDir, "results", "experiments/results", "folder where the report is saved")
	_ = cmd.MarkFlagRequired("config")
	return cmd
}

// newGoldenCmd checks the golden set before any experiment runs.
func newGoldenCmd() *cobra.Command {
	var goldenPath, dataDir string

	cmd := &cobra.Command{
		Use:   "golden",
		Short: "Check that the golden set is valid and its evidence exists in the documents",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			questions, err := eval.LoadQuestions(goldenPath)
			if err != nil {
				return err
			}
			docs, err := ingest.LoadDir(dataDir)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if missing := eval.MissingEvidence(questions, docs); len(missing) > 0 {
				fmt.Fprintln(out, "evidence not found in the documents:")
				for _, m := range missing {
					fmt.Fprintf(out, "  %s\n", m)
				}
				return fmt.Errorf("%d evidence snippet(s) do not match the documents", len(missing))
			}

			fmt.Fprintf(out, "ok  %d questions, every evidence snippet was found in %s\n", len(questions), dataDir)
			return nil
		},
	}

	cmd.Flags().StringVar(&goldenPath, "golden", "data/golden/questions.yaml", "golden set file")
	cmd.Flags().StringVar(&dataDir, "data", "data/raw", "folder with the .txt documents")
	return cmd
}
