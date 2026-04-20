package cmd

import (
	"os"
	"path/filepath"

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
	} else if discovered := discoverConfigFile(); discovered != "" {
		viper.SetConfigFile(discovered)
	}

	viper.SetEnvPrefix("PVT")
	viper.AutomaticEnv()

	_ = viper.ReadInConfig()
}

func discoverConfigFile() string {
	if env := os.Getenv("PVT_CONFIG"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}

	if _, err := os.Stat("pvt.yaml"); err == nil {
		if abs, err := filepath.Abs("pvt.yaml"); err == nil {
			return abs
		}
		return "pvt.yaml"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	paths := []string{
		filepath.Join(home, ".config", "pvt", "config.yaml"),
		filepath.Join(home, ".config", "pvt", "pvt.yaml"),
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}
