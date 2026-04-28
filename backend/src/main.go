package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

func main() {
	// 设置默认端口
	port := os.Getenv("PORT")
	if port == "" {
		port = "19375"
	}

	// 初始化应用
	app, err := NewApplication()
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}
	defer app.Close()

	// 启动后台任务
	backgroundTasks := NewBackgroundTasks(app)
	backgroundTasks.Start()

	// 初始化 Gin 路由
	r := gin.Default()
	router := NewRouter(app)
	router.Register(r)

	// 静态文件服务
	registerStaticFiles(r)

	// 启动服务器
	serverAddr := fmt.Sprintf(":%s", port)
	log.Printf("Server starting on %s", serverAddr)
	if err := r.Run(serverAddr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// registerStaticFiles 注册静态文件服务
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
