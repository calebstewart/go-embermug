package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/adrg/xdg"
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

var cfgFile string

func initConfig() {
	if cfgFile != "" {
		slog.Info("Using configuration file", "Path", cfgFile)
		viper.SetConfigFile(cfgFile)
	} else if cfgFile, err := xdg.ConfigFile("embermug/config.toml"); err == nil {
		slog.Info("Using configuration file", "Path", cfgFile)
		viper.SetConfigFile(cfgFile)
	}

	viper.SetConfigType("toml")
	viper.SetEnvPrefix("EMBER")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "__"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Errorf("fatal error while reading config file: %w", err))
	}
}

func init() {
	rootCmd.AddCommand(&serviceCommand)
	rootCmd.AddCommand(&waybarCommand)

	cobra.OnInitialize(initConfig)

	flags := rootCmd.PersistentFlags()
	flags.StringVar(&cfgFile, "config", "", "config file (default: $XDG_CONFIG_HOME/embermug/config.toml)")

	flags.String("log-level", "info", "Minimum Log Level to Show")
	viper.BindPFlag("log-level", flags.Lookup("log-level"))

	flags.String("socket", "/run/embermug.sock", "Default socket path")
	viper.BindPFlag("socket-path", flags.Lookup("socket"))
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
