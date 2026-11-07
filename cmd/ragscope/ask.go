package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/piovani/RAGscope/internal/config"
	"github.com/piovani/RAGscope/internal/domain"
	"github.com/piovani/RAGscope/internal/generate"
	"github.com/piovani/RAGscope/internal/pipeline"
)

func newAskCmd() *cobra.Command {
	var cfgPath string
	var chunksOnly bool

	cmd := &cobra.Command{
		Use:   "ask <question>",
		Short: "Answer a question about the law, showing the sources used",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			exp, err := config.Load(cfgPath)
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

			var answer pipeline.Answer
			if chunksOnly {
				sources, err := retriever.Retrieve(cmd.Context(), args[0], exp.FinalK())
				if err != nil {
					return err
				}
				answer.Sources = sources
			} else {
				var generator generate.Generator
				generator, err = newGenerator(exp.Generator)
				if err != nil {
					return err
				}
				answerer := pipeline.Answerer{Retriever: retriever, Generator: generator, K: exp.FinalK()}
				answer, err = answerer.Ask(cmd.Context(), args[0])
				if err != nil {
					return err
				}
			}

			out := cmd.OutOrStdout()
			if len(answer.Sources) == 0 {
				fmt.Fprintf(out, "nothing found. Did you run ingest for %q?\n", exp.Collection())
				return nil
			}

			if !chunksOnly {
				if answer.Refused {
					fmt.Fprintln(out, "The retrieved passages do not answer this question.")
				} else {
					fmt.Fprintf(out, "%s\n", answer.Text)
				}
				u := answer.Usage
				fmt.Fprintf(out, "\n(%d tokens in, %d out, %.1fs)\n\n", u.InputTokens, u.OutputTokens, u.Latency.Seconds())
			}

			printSources(out, answer.Sources)
			return nil
		},
	}

	cmd.Flags().StringVarP(&cfgPath, "config", "c", "", "experiment config file")
	cmd.Flags().BoolVar(&chunksOnly, "chunks-only", false, "only show the retrieved passages, without asking the model")
	_ = cmd.MarkFlagRequired("config")
	return cmd
}

// printSources lists the passages in the same order the model saw them, so the
// [1], [2] citations in the answer point to these lines.
func printSources(out io.Writer, sources []domain.ScoredChunk) {
	fmt.Fprintln(out, "Sources:")
	for i, s := range sources {
		fmt.Fprintf(out, "[%d] %.3f %s %s\n    %s\n", i+1, s.Score, s.ID, generate.Label(s.Metadata["article"], s.ID), preview(s.Text, 160))
	}
}

// preview shortens text to at most max characters, without cutting a letter in half.
func preview(text string, max int) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max]) + "..."
}
