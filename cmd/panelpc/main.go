package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"panelpc/internal/audio"
	"panelpc/internal/config"
	"panelpc/internal/device"
	"panelpc/internal/engine"
	"panelpc/internal/server"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8765", "local address for the web interface")
	noBrowser := flag.Bool("no-browser", false, "do not open the browser on startup")
	configOverride := flag.String("config", "", "alternate configuration path")
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
	// Persist generated defaults and migrations, including the integration API
	// token, before accepting requests.
	if err := config.Save(configPath, cfg); err != nil {
		log.Fatalf("save %s: %v", configPath, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dev := device.NewManager()
	aud := audio.New()
	eng := engine.New(dev, aud, cfg)
	go dev.Run(ctx)
	go eng.Run(ctx)

	httpServer := &http.Server{
		Addr:              *listen,
		Handler:           server.New(configPath, cfg, dev, eng, aud).Handler(),
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	url := "http://" + *listen + "/"
	fmt.Printf("PanelPC is ready at %s\n", url)
	if !*noBrowser {
		go func() {
			time.Sleep(150 * time.Millisecond)
			if err := exec.Command("xdg-open", url).Start(); err != nil {
				log.Printf("could not open the browser: %v", err)
			}
		}()
	}

	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.Serve(listener) }()
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server: %v", err)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}
