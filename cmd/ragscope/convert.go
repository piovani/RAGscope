package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/piovani/RAGscope/internal/ingest"
)

func newConvertCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "convert <page.html> <output.txt>",
		Short: "Turn a saved law page (HTML) into clean text, without the revoked parts",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			page, err := os.Open(args[0])
			if err != nil {
				return err
			}
			defer page.Close()

			text, err := ingest.HTMLToText(page)
			if err != nil {
				return err
			}
			if err := os.WriteFile(args[1], []byte(text), 0o644); err != nil {
				return err
			}

			// Counting articles gives a quick feel for whether it worked.
			articles := strings.Count(text, "\nArt.") + strings.Count(text[:min(len(text), 8)], "Art.")
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s (%d words, %d lines starting with \"Art.\")\n",
				args[1], len(strings.Fields(text)), articles)
			return nil
		},
	}
}
