package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/piovani/RAGscope/internal/config"
	"github.com/piovani/RAGscope/internal/ingest"
	"github.com/piovani/RAGscope/internal/pipeline"
)

func newIngestCmd() *cobra.Command {
	var cfgPath, dataDir string

	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Cut the documents into chunks, embed them and save them",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			exp, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			chunker, err := newChunker(exp.Chunking)
			if err != nil {
				return err
			}
			docs, err := ingest.LoadDir(dataDir)
			if err != nil {
				return err
			}

			db, err := openStore(cmd.Context())
			if err != nil {
				return err
			}
			defer db.Close()

			n, err := pipeline.Ingest(cmd.Context(), docs, chunker, newEmbedder(), db, exp.Collection())
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "indexed %d chunks from %d documents into %q\n", n, len(docs), exp.Collection())
			return nil
		},
	}

	cmd.Flags().StringVarP(&cfgPath, "config", "c", "", "experiment config file")
	cmd.Flags().StringVar(&dataDir, "data", "data/raw", "folder with the .txt documents")
	_ = cmd.MarkFlagRequired("config")
	return cmd
}
