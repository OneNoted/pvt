package cmd

import (
	"context"
	"time"

	"github.com/OneNoted/pvt/internal/config"
)

func loadConfig() (string, *config.Config, error) {
	path := cfgFile
	if path == "" {
		discovered, err := config.Discover()
		if err != nil {
			return "", nil, err
		}
		path = discovered
	}
	cfg, err := config.Load(path)
	if err != nil {
		return "", nil, err
	}
	return path, cfg, nil
}

func liveContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}
