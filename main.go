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
	db_path := "/data/db"
	os.MkdirAll(db_path, 0755)
	db_file_path := db_path + "/local_assets.db"
	db, err := bolt.Open(db_file_path, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
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
	defer watcher.Close()
	// 注册api
	api.RegisterApi(r, db, watcher, domain)
	// 启动
	r.Run(":8080")
}
