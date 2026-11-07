package main

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/piovani/RAGscope/internal/api"
	"github.com/piovani/RAGscope/internal/config"
	"github.com/piovani/RAGscope/internal/generate"
	"github.com/piovani/RAGscope/internal/pipeline"
)

func newServeCmd() *cobra.Command {
	var cfgPath, addr string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP API and the web page to ask questions",
		Args:  cobra.NoArgs,
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
			var generator generate.Generator
			generator, err = newGenerator(exp.Generator)
			if err != nil {
				return err
			}

			answerer := pipeline.Answerer{Retriever: retriever, Generator: generator, K: exp.FinalK()}
			srv := &http.Server{
				Addr:              addr,
				Handler:           api.NewHandler(answerer),
				ReadHeaderTimeout: 5 * time.Second,
			}

			errCh := make(chan error, 1)
			go func() { errCh <- srv.ListenAndServe() }()
			slog.Info("api listening", "addr", addr, "experiment", exp.Name)

			select {
			case err := <-errCh:
				if errors.Is(err, http.ErrServerClosed) {
					return nil
				}
				return err
			case <-cmd.Context().Done():
				shutdownCtx, cancel := newShutdownContext()
				defer cancel()
				return srv.Shutdown(shutdownCtx)
			}
		},
	}

	// There is no authentication here: anyone who reaches this port can spend the
	// model's time and read the indexed text. Keep it on localhost and do not
	// expose it to the internet.
	cmd.Flags().StringVarP(&cfgPath, "config", "c", "", "experiment config file")
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:8080", "listen address (local only by default)")
	_ = cmd.MarkFlagRequired("config")
	return cmd
}
