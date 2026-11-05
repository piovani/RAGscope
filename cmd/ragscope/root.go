package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/piovani/RAGscope/internal/api"
	"github.com/piovani/RAGscope/internal/config"
)

// errNotImplemented marks commands that are planned but not built yet.
var errNotImplemented = errors.New("not implemented yet (see the roadmap in README.md)")

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "ragscope",
		Short:         "Benchmark RAG configurations with real metrics",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newIngestCmd(), newEvalCmd(), newServeCmd(), newReportCmd(), newValidateCmd())
	return root
}

func newIngestCmd() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Parse, chunk, embed and index the corpus",
		RunE:  func(*cobra.Command, []string) error { return errNotImplemented },
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", "", "experiment config file")
	return cmd
}

func newEvalCmd() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Run an experiment against the golden set",
		RunE:  func(*cobra.Command, []string) error { return errNotImplemented },
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", "", "experiment config file")
	return cmd
}

func newReportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "report",
		Short: "Generate a comparison report from stored results",
		RunE:  func(*cobra.Command, []string) error { return errNotImplemented },
	}
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
