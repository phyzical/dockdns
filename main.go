package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Tarow/dockdns/internal/config"
	"github.com/Tarow/dockdns/internal/dns"
	"github.com/Tarow/dockdns/internal/provider"
	"github.com/docker/docker/client"
	"github.com/ilyakaznacheev/cleanenv"
)

var configPath string

func main() {
	flag.StringVar(&configPath, "config", "config.yaml", "Path to the configuration file")
	flag.Parse()

	var appCfg config.AppConfig
	err := cleanenv.ReadConfig(configPath, &appCfg)
	if err != nil {
		slog.Error("Failed to read config", "path", configPath, "error", err)
		os.Exit(1)
	}
	slog.SetDefault(getLogger(appCfg.Log))
	slog.Debug("Successfully read config", "config", appCfg)

	providers := map[string]dns.Provider{}
	for _, zone := range appCfg.Zones {
		dnsProvider, err := provider.Get(zone)
		if err != nil {
			slog.Error("Failed to create DNS provider", "zone", zone.Name, "error", err)
			os.Exit(1)
		}
		providers[zone.Name] = dnsProvider
	}

	dockerCli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		dockerCli = nil
		slog.Warn("Could not create docker client, ignoring dynamic configuration", "error", err)
	}

	handler := dns.NewHandler(providers, appCfg.DNS, appCfg.Domains, dockerCli)

	run := func() {
		if err := handler.Run(); err != nil {
			slog.Error("DNS update failed with error", "error", err)
		}
	}

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	slog.Info(fmt.Sprintf("Updating DNS entries every %v seconds", appCfg.Interval))
	run()
	for {
		select {
		case <-time.After(time.Duration(appCfg.Interval) * time.Second):
			run()
		case <-signalCh:
			slog.Info("Received termination signal. Exiting...")
			return
		}
	}
}

func getLogger(cfg config.LogConfig) *slog.Logger {
	var logLevel = parseLogLevel(cfg.Level)
	var handler slog.Handler

	switch cfg.Format {
	case config.LogFormatJson:
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: logLevel,
		})
	case config.LogFormatSimple:
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: logLevel,
		})
	default:
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: logLevel,
		})
	}

	return slog.New(handler)
}

func parseLogLevel(logLevel string) slog.Level {
	switch strings.ToLower(logLevel) {
	case "debug":
		return slog.LevelDebug

	case "info":
		return slog.LevelInfo

	case "warn":
		return slog.LevelWarn

	case "error":
		return slog.LevelError

	default:
		return slog.LevelInfo
	}
}
