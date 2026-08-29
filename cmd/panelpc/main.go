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
	listen := flag.String("listen", "127.0.0.1:8765", "dirección local de la interfaz")
	noBrowser := flag.Bool("no-browser", false, "no abrir el navegador al iniciar")
	configOverride := flag.String("config", "", "ruta alternativa de configuración")
	flag.Parse()

	host, _, err := net.SplitHostPort(*listen)
	if err != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		log.Fatal("-listen debe ser una dirección loopback válida, por ejemplo 127.0.0.1:8765")
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
		log.Fatalf("cargar %s: %v", configPath, err)
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
	fmt.Printf("PanelPC listo en %s\n", url)
	if !*noBrowser {
		go func() {
			time.Sleep(150 * time.Millisecond)
			if err := exec.Command("xdg-open", url).Start(); err != nil {
				log.Printf("no se pudo abrir el navegador: %v", err)
			}
		}()
	}

	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.Serve(listener) }()
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Printf("servidor: %v", err)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}
