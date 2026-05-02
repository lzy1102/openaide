package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"openaide/backend/src/config"
	"openaide/backend/src/middleware"

	"github.com/gin-gonic/gin"
)

const version = "2.0.0"

func main() {
	app, err := NewApplication()
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}

	port := resolvePort(app.Config)

	backgroundTasks := NewBackgroundTasks(app)
	backgroundTasks.Start()

	r := gin.New()
	r.Use(middleware.RequestLogger(), gin.Recovery())
	router := NewRouter(app)
	router.Register(r)

	registerStaticFiles(r)

	serverAddr := fmt.Sprintf(":%s", port)
	srv := &http.Server{
		Addr:    serverAddr,
		Handler: r,
	}

	printBanner(app.Config, port)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	sig := <-quit
	slog.Info("Received shutdown signal", "signal", sig.String())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
	}

	if err := app.Shutdown(ctx); err != nil {
		slog.Error("Error during app shutdown", "error", err)
	}

	slog.Info("Server exited gracefully")
}

func resolvePort(cfg *config.Config) string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	if cfg != nil && cfg.Server.Port > 0 {
		return strconv.Itoa(cfg.Server.Port)
	}
	return "19375"
}

func printBanner(cfg *config.Config, port string) {
	localMode := false
	dbType := "sqlite"
	dbURI := ""
	cacheType := "memory"
	vectorType := "memory"
	configPath := config.GetConfigPath()
	homeDir := ""

	if cfg != nil {
		localMode = cfg.Server.LocalMode || os.Getenv("OPENAIDE_LOCAL_MODE") == "true"
		dbType = cfg.Storage.DB.Type
		dbURI = cfg.Storage.DB.URI
		cacheType = cfg.Storage.Cache.Type
		vectorType = cfg.Storage.VectorStore.Type
		if cfg.HomeDir != "" {
			homeDir = cfg.HomeDir
		}
	}

	if dbType == "" {
		dbType = "sqlite"
	}
	if cacheType == "" {
		cacheType = "memory"
	}
	if vectorType == "" {
		vectorType = "memory"
	}

	authMode := "JWT"
	if localMode {
		authMode = "LOCAL (no auth)"
	}

	fmt.Println()
	fmt.Println("  ╔═══════════════════════════════════════════╗")
	fmt.Println("  ║           OpenAIDE Server v" + version + "          ║")
	fmt.Println("  ╚═══════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("  Version:      %s\n", version)
	fmt.Printf("  Port:         %s\n", port)
	fmt.Printf("  Config:       %s\n", configPath)
	if homeDir != "" {
		fmt.Printf("  Home Dir:     %s\n", homeDir)
	}
	fmt.Printf("  Database:     %s", dbType)
	if dbURI != "" {
		fmt.Printf(" (%s)", dbURI)
	}
	fmt.Println()
	fmt.Printf("  Cache:        %s\n", cacheType)
	fmt.Printf("  Vector Store: %s\n", vectorType)
	fmt.Printf("  Auth Mode:    %s\n", authMode)
	fmt.Println()
	fmt.Printf("  Listening on: http://0.0.0.0:%s\n", port)
	fmt.Println()
}

func registerStaticFiles(r *gin.Engine) {
	frontendDir := os.Getenv("OPENAIDE_FRONTEND_DIR")
	if frontendDir == "" {
		execPath, _ := os.Executable()
		candidates := []string{
			filepath.Join(filepath.Dir(execPath), "frontend"),
			"/usr/share/openaide/frontend",
			"./frontend",
		}
		for _, dir := range candidates {
			if _, err := os.Stat(dir); err == nil {
				frontendDir = dir
				break
			}
		}
	}
	if frontendDir == "" {
		return
	}

	slog.Info("Frontend directory configured", "path", frontendDir)
	r.Static("/src", filepath.Join(frontendDir, "src"))
	r.Static("/public", filepath.Join(frontendDir, "public"))
	faviconPath := filepath.Join(frontendDir, "favicon.ico")
	if _, err := os.Stat(faviconPath); err == nil {
		r.StaticFile("/favicon.ico", faviconPath)
	}

	r.GET("/", func(c *gin.Context) {
		c.File(filepath.Join(frontendDir, "index.html"))
	})

	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		filePath := filepath.Join(frontendDir, path)
		if _, err := os.Stat(filePath); err == nil {
			c.File(filePath)
			return
		}
		c.File(filepath.Join(frontendDir, "index.html"))
	})
}
