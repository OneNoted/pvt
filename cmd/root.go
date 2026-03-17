package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "pvt",
	Short: "Proxmox VE + Talos Linux cluster lifecycle CLI",
	Long: `pvt bridges the Proxmox VM layer and the Talos cluster layer, providing
pre-flight validation, bootstrap orchestration, rolling upgrades,
node lifecycle management, drift detection, and cluster status overview.`,
	SilenceUsage: true,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./pvt.yaml, ~/.config/pvt/config.yaml)")
	rootCmd.PersistentFlags().String("log-level", "info", "log level (debug, info, warn, error)")
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("pvt")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")

		home, err := os.UserHomeDir()
		if err == nil {
			viper.AddConfigPath(fmt.Sprintf("%s/.config/pvt", home))
		}
	}

	viper.SetEnvPrefix("PVT")
	viper.AutomaticEnv()

	_ = viper.ReadInConfig()
}
