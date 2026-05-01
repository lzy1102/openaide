package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "19375"
	}

	app, err := NewApplication()
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}

	backgroundTasks := NewBackgroundTasks(app)
	backgroundTasks.Start()

	r := gin.Default()
	router := NewRouter(app)
	router.Register(r)

	registerStaticFiles(r)

	serverAddr := fmt.Sprintf(":%s", port)
	srv := &http.Server{
		Addr:    serverAddr,
		Handler: r,
	}

	go func() {
		log.Printf("Server starting on %s", serverAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	sig := <-quit
	log.Printf("Received signal: %v, shutting down...", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	if err := app.Shutdown(ctx); err != nil {
		log.Printf("Error during app shutdown: %v", err)
	}

	log.Println("Server exited gracefully")
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

	log.Printf("Frontend directory: %s", frontendDir)
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
