package main

import (
	"github.com/gin-gonic/gin"

	"random_local_asset_api/api"
)

func main() {
	r := gin.New()
	api.RegisterApi(r)
	r.Run()
}
