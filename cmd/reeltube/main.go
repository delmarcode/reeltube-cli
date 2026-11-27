package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/the-del-mar-code-company/reeltube-cli/internal/cli"
	"github.com/the-del-mar-code-company/reeltube-cli/internal/client"
)

var rootCmd = &cobra.Command{
	Use:   "reeltube",
	Short: "Reeltube is a CLI for interacting with the Reeltube API",
}

func main() {
	var apiKey, baseURL string
	var debug bool

	rootCmd.PersistentFlags().StringVarP(&apiKey, "api-key", "k", "", "API key for authentication (required unless REELTUBE_API_KEY is set)")
	rootCmd.PersistentFlags().StringVarP(&baseURL, "base-url", "u", "https://api.reel.tube", "Base URL of the API")
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "Enable debug mode")

	_ = godotenv.Load() // Load .env file if present

	// Add commands
	rootCmd.AddCommand(cli.NewWhoamiCmd())
	rootCmd.AddCommand(cli.NewVersionCmd())
	rootCmd.AddCommand(cli.NewUploadCmd())
	rootCmd.AddCommand(cli.NewListCmd())
	rootCmd.AddCommand(cli.NewDeleteCmd())

	// Set up context with signal handling for graceful cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Fprintln(os.Stderr, "\nInterrupted, cancelling... (press Ctrl-C again to quit now)")
		cancel()
		// Hand later signals back to the runtime, so a second Ctrl-C ends the process
		// even if something ignores the cancelled context.
		signal.Stop(sigChan)
	}()

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Skip API key check for version command
		if cmd.Use == "version" {
			return nil
		}

		// Load config file (lowest priority, can be overridden)
		fileConfig, err := cli.LoadConfigFile()
		if err != nil {
			return fmt.Errorf("failed to load config file: %w", err)
		}

		// Priority: flags > env vars > config file > defaults
		// Determine API key
		key := apiKey
		if key == "" {
			key = os.Getenv("REELTUBE_API_KEY")
		}
		if key == "" && fileConfig != nil {
			key = fileConfig.APIKey
		}
		if key == "" {
			return fmt.Errorf("API key must be provided via --api-key flag, REELTUBE_API_KEY environment variable, or config file")
		}

		// Determine base URL (flag > env > config > default)
		url := baseURL
		if !cmd.Flags().Changed("base-url") {
			// Flag wasn't explicitly set, check other sources
			if envURL := os.Getenv("REELTUBE_API_HOST"); envURL != "" {
				url = envURL
			} else if fileConfig != nil && fileConfig.BaseURL != "" {
				url = fileConfig.BaseURL
			}
		}

		// Create logger based on debug flag
		var logger *slog.Logger
		if debug {
			logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
		} else {
			logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
		}

		cfg := &cli.Config{
			APIKey:  key,
			BaseURL: url,
			Debug:   debug,
			Logger:  logger,
			Client:  client.NewClient(url, key, logger),
		}

		cmd.SetContext(context.WithValue(ctx, cli.SetConfigKey(), cfg))
		return nil
	}

	// Commands return their errors; print each once here and exit nonzero.
	rootCmd.SilenceErrors = true
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
