package main

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/arkadyb/climate_mate/internal/pkg/app"
	"github.com/arkadyb/climate_mate/internal/pkg/config"
	"github.com/arkadyb/climate_mate/internal/server"
	joonix "github.com/joonix/log"
	log "github.com/sirupsen/logrus"
)

var (
	version = "undefined"
)

func main() {
	cfg := new(config.Config)
	cfg.Init()

	if cfg.LogFormat == strings.ToLower("gcp") {
		log.SetFormatter(joonix.NewFormatter())
	}

	log.WithFields(log.Fields{
		"version": version,
	}).Info("build information")

	server := server.NewServer(
		version,
		cfg.Port,
		app.NewApp(context.Background(), *cfg),
	)
	server.Start()

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	signal := <-c

	server.Stop()
	log.Fatalf("Process killed with signal: %v", signal.String())
}
