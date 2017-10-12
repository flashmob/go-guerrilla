package main

import (
	"github.com/spf13/cobra"

	guerrilla "github.com/flashmob/go-guerrilla"
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
	mainlog.Infof("guerrillad %s", guerrilla.Version)
	mainlog.Debugf("Build Time: %s", guerrilla.BuildTime)
	mainlog.Debugf("Commit:     %s", guerrilla.Commit)
}
