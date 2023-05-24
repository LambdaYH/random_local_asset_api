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

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	bolt "go.etcd.io/bbolt"
)

var buckets map[string]int = make(map[string]int)

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
					data := make([]LocalData, count)
					if count == 1 {
						data[0] = LocalData{Url: buildURL(domain, b.Get(itob(items[0])))}
					} else {
						var wait_group sync.WaitGroup
						for idx, item_id := range items {
							wait_group.Add(1)
							go func(idx int, item_id int) {
								data[idx] = LocalData{Url: buildURL(domain, b.Get(itob(item_id)))}
								wait_group.Done()
							}(idx, item_id)
						}
						wait_group.Wait()
					}
					c.JSON(http.StatusOK, RetJson{Code: 1, Data: data})
				} else if ret_type == "file" {
					// file时候直接重定向，忽略数量
					c.Redirect(http.StatusSeeOther, buildURL(domain, b.Get(itob(items[0]))))
				}
				return nil
			})
		} else {
			c.JSON(http.StatusOK, gin.H{"code": 0, "error": "没有这个文件夹"})
		}
	}
}

func loadLocalAssets(assets_dir string, db *bolt.DB, domain string) {
	dirs, err := os.ReadDir(assets_dir)
	if err != nil {
		panic(err)
	}
	for _, dir := range dirs {
		if dir.IsDir() {
			// 每个文件夹对应一个bucket
			//
			db.Update(func(tx *bolt.Tx) error {
				bucket, err := tx.CreateBucketIfNotExists([]byte(dir.Name()))
				if err != nil {
					return fmt.Errorf("create bucket: %s", err)
				}
				filepath.WalkDir(assets_dir+dir.Name(), func(path string, di fs.DirEntry, err error) error {
					file, _ := os.Stat(path)
					if !file.IsDir() {
						id, _ := bucket.NextSequence()
						bucket.Put(ui64tob(id), []byte(path))
					}
					return nil
				})
				// 记录最大id
				id, _ := bucket.NextSequence()
				buckets[dir.Name()] = int(id) - 1
				return nil
			})
		}
	}
}

func RegisterApi(r *gin.Engine, db *bolt.DB, domain string) {
	api_group := r.Group("/api")

	// === 本地资源api ===
	assets_dir := "/assets/"
	// 设置静态资源路径
	r.Static("/assets", assets_dir)
	// 加载本地资源
	loadLocalAssets(assets_dir, db, domain)
	if reload := os.Getenv("RELOAD"); reload != "" {
		// 重新加载数据库
		log.Println("已配置本地资源重载：" + reload)
		c := cron.New()
		c.AddFunc("*/1 * * * *", func() {
			log.Println("开始重新加载本地资源")
			db.Update(func(tx *bolt.Tx) error {
				for bucket := range buckets {
					tx.DeleteBucket([]byte(bucket))
				}
				return nil
			})
			buckets = make(map[string]int)
			loadLocalAssets(assets_dir, db, domain)
			log.Println("本地资源重载完成")
		})
		c.Start()
	}

	api_group.GET("/assets", assetsApiHandler(domain, db))
}
