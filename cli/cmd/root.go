package cmd

import (
	"log/slog"
	"os"

	"github.com/phsym/console-slog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

var rootCmd = cobra.Command{
	Use:   "embermug",
	Short: "Ember Mug client and server entrypoint",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return configureLogging()
	},
}

func init() {
	rootCmd.AddCommand(&serviceCommand)
	rootCmd.AddCommand(&waybarCommand)

	rootCmd.PersistentFlags().String("log-level", "info", "Minimum Log Level to Show")
	rootCmd.PersistentFlags().String("socket", "/run/embermug.sock", "Default socket path")
	viper.BindPFlags(rootCmd.PersistentFlags())
}

func configureLogging() error {
	var level slog.Level

	if err := level.UnmarshalText([]byte(viper.GetString("log-level"))); err != nil {
		return err
	}

	if term.IsTerminal(int(os.Stderr.Fd())) {
		// Setup the default global logger
		slog.SetDefault(
			slog.New(console.NewHandler(os.Stderr, &console.HandlerOptions{
				Level: level,
			})),
		)
	} else {
		slog.SetDefault(
			slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: level,
			})),
		)
	}

	return nil
}

func commandExitWrapper(entrypoint func(*cobra.Command, []string) error) func(*cobra.Command, []string) {
	return func(cmd *cobra.Command, args []string) {
		if entrypoint(cmd, args) != nil {
			os.Exit(1)
		}
	}
}

func Execute() error {
	return rootCmd.Execute()
}
