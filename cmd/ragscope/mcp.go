package main

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/piovani/RAGscope/internal/config"
	"github.com/piovani/RAGscope/internal/mcp"
	"github.com/piovani/RAGscope/internal/pipeline"
)

func newMCPCmd() *cobra.Command {
	var cfgPath string
	var retrievalOnly bool

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Serve RAGscope as an MCP tool over stdio, for Claude, Copilot, Cursor and any MCP client",
		Long: "Starts a Model Context Protocol server on stdin and stdout. The assistant that launches it\n" +
			"gets a search tool that returns passages of the law and, unless --retrieval-only is set,\n" +
			"an ask tool that also writes the answer with the experiment's model.\n\n" +
			"With --retrieval-only no model is needed at all: the assistant's own model writes the answer.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
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

			answerer := pipeline.Answerer{Retriever: retriever, K: exp.FinalK()}
			server := &mcp.Server{Searcher: answerer, Version: "1.0.0"}
			if !retrievalOnly {
				generator, err := newGenerator(exp.Generator)
				if err != nil {
					return err
				}
				answerer.Generator = generator
				server.Asker = answerer
			}

			// stdout carries the protocol, so logs go to stderr.
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
			slog.Info("mcp server ready", "experiment", exp.Name, "ask_tool", !retrievalOnly)
			return server.Serve(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVarP(&cfgPath, "config", "c", "", "experiment config file")
	cmd.Flags().BoolVar(&retrievalOnly, "retrieval-only", false, "offer only the search tool, so no generation model is needed")
	_ = cmd.MarkFlagRequired("config")
	return cmd
}
