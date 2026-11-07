package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/piovani/RAGscope/internal/eval"
)

// newGradeSheetCmd writes the sheet a person fills in to grade the answers.
func newGradeSheetCmd() *cobra.Command {
	var answersPath, outPath, goldenPath string
	var n int

	cmd := &cobra.Command{
		Use:   "grade-sheet",
		Short: "Write a YAML sheet for a person to grade a sample of the answers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if n <= 0 {
				return errors.New("--n must be greater than zero")
			}
			report, err := eval.LoadAnswerReport(answersPath)
			if err != nil {
				return err
			}
			questions, err := eval.LoadQuestions(goldenPath)
			if err != nil {
				return err
			}

			entries := eval.BuildSheet(report, questions, n)
			if len(entries) == 0 {
				return errors.New("the report has no real answers to answerable questions")
			}
			if err := eval.WriteSheet(outPath, entries); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "saved: %s (%d entries to grade)\n", outPath, len(entries))
			return nil
		},
	}

	cmd.Flags().StringVar(&answersPath, "answers", "", "answer report (.json) made by eval-answers")
	cmd.Flags().StringVar(&outPath, "out", "", "sheet file to write (.yaml)")
	cmd.Flags().StringVar(&goldenPath, "golden", "data/golden/questions.yaml", "golden set file")
	cmd.Flags().IntVar(&n, "n", 30, "how many answers to put in the sheet")
	_ = cmd.MarkFlagRequired("answers")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}

// newCalibrateCmd compares the person's grades with the judge's verdicts.
func newCalibrateCmd() *cobra.Command {
	var answersPath, sheetPath string

	cmd := &cobra.Command{
		Use:   "calibrate",
		Short: "Measure how often the answer judge agrees with a person's grades",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := eval.LoadAnswerReport(answersPath)
			if err != nil {
				return err
			}
			sheet, err := eval.ReadSheet(sheetPath)
			if err != nil {
				return err
			}

			result := eval.Calibrate(report, sheet)
			fmt.Fprint(cmd.OutOrStdout(), eval.RenderCalibration(result))
			return nil
		},
	}

	cmd.Flags().StringVar(&answersPath, "answers", "", "answer report (.json) with the judge's verdicts")
	cmd.Flags().StringVar(&sheetPath, "sheet", "", "sheet (.yaml) filled in by a person")
	_ = cmd.MarkFlagRequired("answers")
	_ = cmd.MarkFlagRequired("sheet")
	return cmd
}
