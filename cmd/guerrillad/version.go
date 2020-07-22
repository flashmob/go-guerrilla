package main

import (
	"github.com/spf13/cobra"

	"github.com/flashmob/go-guerrilla"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version info",
	Long:  `Every software has a version. This is Guerrilla's`,
	Run: func(cmd *cobra.Command, args []string) {
		logVersion()
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

func logVersion() {
	mainlog.Fields(
		"version", guerrilla.Version,
		"buildTime", guerrilla.BuildTime,
		"commit", guerrilla.Commit).
		Info("guerrillad")
}
