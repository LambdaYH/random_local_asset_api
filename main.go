package main

import (
	"log"
	"os"

	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"
	bolt "go.etcd.io/bbolt"

	"random_local_asset_api/api"
)

func main() {
	r := gin.New()

	// 打开数据库
	dbPath := "/data/db"
	err := os.MkdirAll(dbPath, 0755)
	if err != nil {
		log.Fatal("目录创建失败", err)
		return
	}
	dbFilePath := dbPath + "/local_assets.db"
	db, err := bolt.Open(dbFilePath, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer func(db *bolt.DB) {
		err := db.Close()
		if err != nil {

		}
	}(db)
	// 获取配置的域名，返回链接用
	domain := os.Getenv("DOMAIN")
	if domain == "" {
		domain = "http://127.0.0.1:8080"
	} else if domain[len(domain)-1] == '/' {
		domain = domain[:len(domain)-1]
	}
	// 打开个watcher监听文件变动
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}
	defer func(watcher *fsnotify.Watcher) {
		err := watcher.Close()
		if err != nil {

		}
	}(watcher)
	// 中间件
	r.Use(corsMiddleware())
	// 注册api
	api.RegisterApi(r, db, watcher, domain)
	// 启动
	err = r.Run(":8080")
	if err != nil {
		log.Fatal("启动失败", err)
		return
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 允许所有来源的请求
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		// 允许特定的HTTP方法
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		// 允许特定的HTTP头部字段
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		// 如果是预检请求（OPTIONS），则直接返回200状态码
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(200)
			return
		}

		// 继续处理请求
		c.Next()
	}
}
