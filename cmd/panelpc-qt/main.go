//go:build qt

package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"

	"panelpc/internal/audio"
	"panelpc/internal/config"
	"panelpc/internal/device"
	"panelpc/internal/engine"
	"panelpc/internal/qtui"
	"panelpc/internal/server"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8765", "loopback address for the integration API")
	configOverride := flag.String("config", "", "alternate configuration path")
	noHardware := flag.Bool("no-hardware", false, "preview the interface without opening the PCPanel")
	noAPI := flag.Bool("no-api", false, "disable the local integration API (intended for interface previews)")
	background := flag.Bool("background", false, "start hidden in the system tray")
	screenshotDir := flag.String("screenshots", "", "write documentation screenshots to this directory and exit")
	flag.Parse()

	host, _, err := net.SplitHostPort(*listen)
	if err != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		log.Fatal("-listen must be a valid loopback address, for example 127.0.0.1:8765")
	}
	configPath := *configOverride
	if configPath == "" {
		configPath, err = config.Path()
		if err != nil {
			log.Fatal(err)
		}
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load %s: %v", configPath, err)
	}
	if err := config.Save(configPath, cfg); err != nil {
		log.Fatalf("save %s: %v", configPath, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dev := device.NewManager()
	aud := audio.New()
	eng := engine.New(dev, aud, cfg)
	controller := server.New(configPath, cfg, dev, eng, aud)
	preview := *noHardware || *screenshotDir != ""
	if !preview {
		go dev.Run(ctx)
		go eng.Run(ctx)
	}

	// flag.Parse does not remove application-specific options from os.Args.
	// Keep them away from Qt's own command-line parser.
	app := qt.NewQApplication([]string{os.Args[0]})
	app.SetStyleSheet(qtui.StyleSheet)
	window := qtui.NewWindow(controller, dev, eng, aud, *listen, preview)
	if *screenshotDir != "" {
		if err := window.SaveScreenshots(*screenshotDir); err != nil {
			log.Fatal(err)
		}
		return
	}
	if !*background || !window.TrayAvailable() {
		window.Show()
	}

	var httpServer *http.Server
	if !*noAPI {
		var listener net.Listener
		httpServer, listener = startHTTP(*listen, controller.Handler())
		go func() {
			if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("integration API: %v", err)
				mainthread.Start(qt.QCoreApplication_Quit)
			}
		}()
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-signals:
			mainthread.Start(qt.QCoreApplication_Quit)
		case <-ctx.Done():
		}
	}()
	qt.QApplication_Exec()
	signal.Stop(signals)
	cancel()
	if httpServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}
}

func startHTTP(address string, handler http.Handler) (*http.Server, net.Listener) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("listen on %s: %v", address, err)
	}
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}, listener
}
