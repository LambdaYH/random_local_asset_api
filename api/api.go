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

// 首次加载资源
func loadLocalAssets(assets_dir string, db *bolt.DB, watcher *fsnotify.Watcher) {
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
					} else {
						// 子路径加入watcher监听中
						watcher.Add(path)
					}
					return nil
				})
				// 记录最大id
				id, _ := bucket.NextSequence()
				buckets[dir.Name()] = int(id) - 1
				// 监听文件变动
				watcher.Add(assets_dir + dir.Name())
				return nil
			})
		}
	}
}

// 文件变动时重载bucket
func reloadBucket(assets_dir string, db *bolt.DB, watcher *fsnotify.Watcher, bucket_name string) {
	db.Update(func(tx *bolt.Tx) error {
		tx.DeleteBucket([]byte(bucket_name))
		bucket, err := tx.CreateBucketIfNotExists([]byte(bucket_name))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		filepath.WalkDir(assets_dir+bucket_name, func(path string, di fs.DirEntry, err error) error {
			file, err := os.Stat(path)
			if err != nil {
				return err
			}
			if !file.IsDir() {
				id, _ := bucket.NextSequence()
				bucket.Put(ui64tob(id), []byte(path))
			}
			return nil
		})
		// 记录最大id
		id, _ := bucket.NextSequence()
		buckets[bucket_name] = int(id) - 1
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
				log.Println("文件发生变化:", event)
				if event.Has(fsnotify.Create) {
					// 如果是文件夹，加入监听
					file, _ := os.Stat(event.Name)
					if file.IsDir() {
						watcher.Add(event.Name)
					}
				}
				if event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) || event.Has(fsnotify.Write) {
					splits := strings.Split(event.Name, "/")
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
							buckets_timer[bucket_name] = time.NewTimer(5 * time.Minute)
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
	api_group.GET("/assets", assetsApiHandler(domain, db))
}
