package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/piovani/RAGscope/internal/config"
	"github.com/piovani/RAGscope/internal/retrieve"
)

// For now "ask" only shows what was retrieved. The answer from the LLM comes
// in the next step.
func newAskCmd() *cobra.Command {
	var cfgPath string

	cmd := &cobra.Command{
		Use:   "ask <question>",
		Short: "Show the chunks retrieved for a question",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			exp, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			if exp.Retrieval.Mode != "vector" {
				return fmt.Errorf("retrieval mode %q is not implemented yet (only \"vector\")", exp.Retrieval.Mode)
			}

			db, err := openStore(cmd.Context())
			if err != nil {
				return err
			}
			defer db.Close()

			retriever := retrieve.Vector{Embedder: newEmbedder(), Searcher: db, Collection: exp.Name}
			results, err := retriever.Retrieve(cmd.Context(), args[0], exp.Retrieval.K)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if len(results) == 0 {
				fmt.Fprintf(out, "nothing found. Did you run ingest for %q?\n", exp.Name)
				return nil
			}
			for i, r := range results {
				fmt.Fprintf(out, "%d. [%.3f] %s\n   %s\n\n", i+1, r.Score, r.ID, preview(r.Text, 200))
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&cfgPath, "config", "c", "", "experiment config file")
	_ = cmd.MarkFlagRequired("config")
	return cmd
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
