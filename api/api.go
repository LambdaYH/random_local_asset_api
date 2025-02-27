package api

import (
	"encoding/binary"
	"fmt"
	"io/fs"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"
	bolt "go.etcd.io/bbolt"
)

var buckets map[string]int = make(map[string]int)
var buckets_timer map[string]*time.Timer = make(map[string]*time.Timer)

type RetJson struct {
	Code  int         `json:"code"`
	Error string      `json:"error,omitempty"`
	Data  interface{} `json:"data,omitempty"`
}

type LocalData struct {
	Url string `json:"url"`
}

func ui64tob(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

func itob(v int) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

func buildURL(domain string, filepath []byte) string {
	var build strings.Builder
	build.WriteString(domain)
	build.Write(filepath)
	return build.String()
}

// 返回随机资源
func assetsApiHandler(domain string, db *bolt.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ret_type := c.DefaultQuery("type", "json")
		category := c.Query("category")
		count_param := c.DefaultQuery("count", "1")
		count, err := strconv.Atoi(count_param)
		if err != nil {
			c.JSON(http.StatusOK, RetJson{Code: 0, Error: "count参数必须为数字"})
			return
		}
		if count >= 100 {
			c.JSON(http.StatusOK, RetJson{Code: 0, Error: "count不能超过100"})
			return
		}
		if count <= 0 {
			c.JSON(http.StatusOK, RetJson{Code: 0, Error: "count必须为正整数"})
			return
		}
		if _, ok := buckets[category]; ok {
			// 记录文件的id
			items := make([]int, count)
			item_total_count := buckets[category]
			if item_total_count < count {
				c.JSON(http.StatusOK, RetJson{Code: 0, Error: "目录下不存在足够的文件"})
				return
			}
			selected_items := make(map[int]struct{})
			for i := 0; i < count; i++ {
				random_idx := rand.Intn(item_total_count) + 1
				if _, ok := selected_items[random_idx]; ok {
					// 重新选一次，因为这个已经被选了
					i--
				}
				selected_items[random_idx] = struct{}{}
				items[i] = random_idx
			}
			db.View(func(tx *bolt.Tx) error {
				b := tx.Bucket([]byte(category))
				if ret_type == "json" {
					// json时候正常返回数量
					if c.Request.Method == "HEAD" {
						c.Header("Content-Type", "application/json")
						c.Status(http.StatusOK)
						return nil
					}
					data := make([]LocalData, count)
					if count == 1 {
						filePath := b.Get(itob(items[0]))
						if filePath == nil {
							c.JSON(http.StatusOK, RetJson{Code: 0, Error: "文件路径未找到"})
							return nil
						}
						data[0] = LocalData{Url: buildURL(domain, filePath)}
					} else {
						var wait_group sync.WaitGroup
						for idx, item_id := range items {
							wait_group.Add(1)
							go func(idx int, item_id int) {
								filePath := b.Get(itob(item_id))
								if filePath != nil {
									data[idx] = LocalData{Url: buildURL(domain, filePath)}
								} else {
									data[idx] = LocalData{Url: ""}
								}
								wait_group.Done()
							}(idx, item_id)
						}
						wait_group.Wait()
					}
					c.JSON(http.StatusOK, RetJson{Code: 1, Data: data})
				} else if ret_type == "file" {
					// file时候直接重定向，忽略数量
					if c.Request.Method == "HEAD" {
						c.Header("Location", buildURL("/", b.Get(itob(items[0]))))
						c.Status(http.StatusSeeOther)
						return nil
					}
					c.Redirect(http.StatusSeeOther, buildURL("/", b.Get(itob(items[0]))))
				}
				return nil
			})
		} else {
			c.JSON(http.StatusOK, gin.H{"code": 0, "error": "没有这个文件夹"})
		}
	}
}

func processDirectory(assets_dir string, db *bolt.DB, watcher *fsnotify.Watcher, bucket_name string, bucket *bolt.Bucket, extensions []string) error {
	filepath.WalkDir(filepath.Join(assets_dir, bucket_name), func(path string, di fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// 跳过根目录自身
		if path == filepath.Join(assets_dir, bucket_name) {
			return nil
		}
		file, err := os.Stat(path)
		if err != nil {
			return err
		}
		if !file.IsDir() {
			// 检查文件后缀是否在 extensions 列表中
			ext := filepath.Ext(path)
			if len(extensions) > 0 && !contains(extensions, ext) {
				return nil
			}
			id, _ := bucket.NextSequence()
			bucket.Put(ui64tob(id), []byte(path))
		} else {
			// 子路径加入watcher监听中
			watcher.Add(path)
		}
		return nil
	})
	// 记录最大id
	id, _ := bucket.NextSequence()
	buckets[bucket_name] = int(id) - 1
	return nil
}

// 首次加载资源
func loadLocalAssets(assets_dir string, db *bolt.DB, watcher *fsnotify.Watcher) {
	// 读取环境变量 FILE_EXTENSIONS
	fileExtensions := os.Getenv("FILE_EXTENSIONS")
	var extensions []string
	if fileExtensions != "" {
		extensions = strings.Split(fileExtensions, ",")
	}
	if len(extensions) > 0 {
		log.Println("已加载文件后缀过滤，仅处理", strings.Join(extensions, ", "))
	}

	dirs, err := os.ReadDir(assets_dir)
	if err != nil {
		panic(err)
	}
	for _, dir := range dirs {
		if dir.IsDir() {
			// 每个文件夹对应一个bucket
			db.Update(func(tx *bolt.Tx) error {
				// 删除已存在的桶
				if err := tx.DeleteBucket([]byte(dir.Name())); err != nil && err != bolt.ErrBucketNotFound {
					return fmt.Errorf("delete bucket: %s", err)
				}
				bucket, err := tx.CreateBucket([]byte(dir.Name()))
				if err != nil {
					return fmt.Errorf("create bucket: %s", err)
				}
				err = processDirectory(assets_dir, db, watcher, dir.Name(), bucket, extensions)
				if err != nil {
					return err
				}
				// 监听文件变动
				watcher.Add(filepath.Join(assets_dir, dir.Name()))
				return nil
			})
		}
	}
}

// 文件变动时重载bucket
func reloadBucket(assets_dir string, db *bolt.DB, watcher *fsnotify.Watcher, bucket_name string) {
	// 读取环境变量 FILE_EXTENSIONS
	fileExtensions := os.Getenv("FILE_EXTENSIONS")
	var extensions []string
	if fileExtensions != "" {
		extensions = strings.Split(fileExtensions, ",")
	}
	if len(extensions) > 0 {
		log.Println("已加载文件后缀过滤，仅处理", strings.Join(extensions, ", "))
	}

	db.Update(func(tx *bolt.Tx) error {
		tx.DeleteBucket([]byte(bucket_name))
		bucket, err := tx.CreateBucketIfNotExists([]byte(bucket_name))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		err = processDirectory(assets_dir, db, watcher, bucket_name, bucket, extensions)
		if err != nil {
			return err
		}
		return nil
	})
}

func RegisterApi(r *gin.Engine, db *bolt.DB, watcher *fsnotify.Watcher, domain string) {
	api_group := r.Group("/api")

	// === 本地资源api ===
	assets_dir := "/assets/"
	// 设置静态资源路径
	r.Static("/assets", assets_dir)
	// 加载本地资源
	loadLocalAssets(assets_dir, db, watcher)
	// 启用watcher监听
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Create) {
					// 如果是文件夹，加入监听
					file, _ := os.Stat(event.Name)
					if file != nil && file.IsDir() {
						watcher.Add(event.Name)
					}
				}
				if event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) {
					log.Println("文件发生变化:", event)
					// 修改: 使用 filepath.Separator 替换 "/"
					splits := strings.Split(event.Name, string(filepath.Separator))
					bucket_name := splits[2]
					// 如果是删除一个类别，那么把桶也删了
					if event.Has(fsnotify.Remove) {
						if len(splits) == 3 {
							delete(buckets, bucket_name)
							delete(buckets_timer, bucket_name)
							db.Update(func(tx *bolt.Tx) error {
								tx.DeleteBucket([]byte(bucket_name))
								return nil
							})
							log.Println("已清除文件夹：" + bucket_name)
						}
					} else {
						if timer, ok := buckets_timer[bucket_name]; !ok || timer == nil {
							// 没有定时任务，创建5min的一个延时定时器
							buckets_timer[bucket_name] = time.NewTimer(1 * time.Minute)
							// 创建重载桶的任务
							go func(bucket_name string, timer *time.Timer) {
								<-timer.C
								log.Println("开始重载文件夹：" + bucket_name)
								reloadBucket(assets_dir, db, watcher, bucket_name)
								log.Println("文件夹重载完成：" + bucket_name)
								// 删除定时器
								buckets_timer[bucket_name] = nil
							}(bucket_name, buckets_timer[bucket_name])
						} else {
							// 说明已经存在了一个定时器，把他重置
							if !buckets_timer[bucket_name].Stop() {
								<-buckets_timer[bucket_name].C
							}
							buckets_timer[bucket_name].Reset(5 * time.Minute)
						}
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)
			}
		}
	}()
	watcher.Add(assets_dir)
	// 修改: 同时支持 GET 和 HEAD 方法
	api_group.Handle("GET", "/assets", assetsApiHandler(domain, db))
	api_group.Handle("HEAD", "/assets", assetsApiHandler(domain, db))

	// 添加新的路由处理 /api 请求
	api_group.GET("/", func(c *gin.Context) {
		if len(c.Request.URL.Query()) == 0 {
			var categories []string
			db.View(func(tx *bolt.Tx) error {
				return tx.ForEach(func(name []byte, _ *bolt.Bucket) error {
					categories = append(categories, string(name))
					return nil
				})
			})
			// 设置响应头为 text/html 并指定 UTF-8 编码
			c.Header("Content-Type", "text/html; charset=utf-8")

			// 构建包含 <meta charset="UTF-8"> 的 HTML 响应
			htmlContent := fmt.Sprintf(
				"<html><head><meta charset=\"UTF-8\"></head><body><h1>使用说明</h1>"+
					"<p>访问 /api/assets?category=<category_name>&count=<number> 来获取随机资源</p>"+
					"<h2>类别列表</h2><ul><li>%s</li></ul></body></html>",
				strings.Join(categories, "</li><li>"),
			)
			c.String(http.StatusOK, htmlContent)
		} else {
			c.Next()
		}
	})
}

// 添加 contains 函数用于检查切片中是否包含某个元素
func contains(slice []string, element string) bool {
	for _, elem := range slice {
		if elem == element {
			return true
		}
	}
	return false
}
