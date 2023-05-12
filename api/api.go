package api

import (
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

var category_pic map[string][]string = make(map[string][]string)
var category_folder map[string]string = make(map[string]string)

type ConfigItem struct {
	Folder   string   `yaml:"folder"`
	Category string   `yaml:"category"`
	Suffixes []string `yaml:"suffixes,flow"`
}

type RetJson struct {
	Code  int         `json:"code"`
	Error string      `json:"error,omitempty"`
	Data  interface{} `json:"data,omitempty"`
}

type ImageData struct {
	Url string `json:"url"`
}

func loadConfig() []ConfigItem {
	config_path := "/config/config.yaml"
	config_file, err := os.ReadFile(config_path)
	if err != nil {
		panic(err)
	}
	var configs []ConfigItem
	if err := yaml.Unmarshal(config_file, &configs); err != nil {
		panic(err)
	}
	return configs
}

func loadImages(configs []ConfigItem) {
	for _, config := range configs {
		category_folder[config.Category] = config.Folder
		all_files, err := os.ReadDir(config.Folder)
		if err != nil {
			panic(err)
		}
		suffixes_map := make(map[string]struct{})
		for _, suffix := range config.Suffixes {
			suffixes_map[suffix] = struct{}{}
		}
		for _, file := range all_files {
			if _, ok := suffixes_map[filepath.Ext(file.Name())]; ok {
				category_pic[config.Category] = append(category_pic[config.Category], file.Name())
			}
		}
	}
}

// 返回随机资源
func assetsApiHandler(c *gin.Context) {
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
	if _, ok := category_folder[category]; ok {
		imgs := make([]string, count)
		len_cate := len(category_pic[category])
		if len_cate < count {
			c.JSON(http.StatusOK, RetJson{Code: 0, Error: "目录下不存在足够的图片"})
			return
		}
		for i := 0; i < count; i++ {
			imgs[i] = category_pic[category][rand.Intn(len_cate)]
		}
		if ret_type == "json" {
			// json时候正常返回数量
			data := make([]ImageData, count)
			for i, img := range imgs {
				data[i] = ImageData{Url: "http://127.0.0.1:8080/assets/" + category + "/" + img}
			}
			c.JSON(http.StatusOK, RetJson{Code: 1, Data: data})
		} else if ret_type == "file" {
			// img时候直接重定向，忽略数量
			c.Redirect(http.StatusSeeOther, "http://127.0.0.1:8080/assets/"+category+"/"+imgs[0])
		}
	} else {
		c.JSON(http.StatusOK, gin.H{"code": 0, "error": "没有这个图片类别"})
	}
}

func RegisterApi(r *gin.Engine) {
	configs := loadConfig()
	loadImages(configs)
	api_group := r.Group("/api")
	for _, config := range configs {
		r.Static("/assets/"+config.Category, config.Folder)
	}

	// 图片api
	api_group.GET("/assets", assetsApiHandler)
}
