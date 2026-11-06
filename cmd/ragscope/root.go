package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/piovani/RAGscope/internal/api"
	"github.com/piovani/RAGscope/internal/config"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "ragscope",
		Short:         "Benchmark RAG configurations with real metrics",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newIngestCmd(), newAskCmd(), newEvalCmd(), newGoldenCmd(), newConvertCmd(), newServeCmd(), newReportCmd(), newValidateCmd())
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

func newServeCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP API",
		RunE: func(cmd *cobra.Command, _ []string) error {
			srv := &http.Server{
				Addr:              addr,
				Handler:           api.NewHandler(),
				ReadHeaderTimeout: 5 * time.Second,
			}

			errCh := make(chan error, 1)
			go func() { errCh <- srv.ListenAndServe() }()
			slog.Info("api listening", "addr", addr)

			select {
			case err := <-errCh:
				return err
			case <-cmd.Context().Done():
				shutdownCtx, cancel := newShutdownContext()
				defer cancel()
				return srv.Shutdown(shutdownCtx)
			}
		},
	}
	cmd.Flags().StringVar(&addr, "addr", ":8080", "listen address")
	return cmd
}
