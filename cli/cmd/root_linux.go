package cmd

import "github.com/spf13/viper"

func init() {
	rootCmd.PersistentFlags().String("socket", "/run/embermug.sock", "Default Unix Socket Path")
	viper.BindPFlag("socket-path", rootCmd.PersistentFlags().Lookup("socket"))
}
