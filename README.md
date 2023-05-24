一个返回随机本地资源的简陋API

# 部署方法

1. 创建个目录`folder`
2. 
```
cd folder
wget https://raw.githubusercontent.com/LambdaYH/random_local_asset_api/main/docker-compose.yml
```
3. 填写docker-compose.yml
假如本地文件夹为`/srv/local_dir/`，当访问`host/api/assets/category=local`返回此文件夹内的随机图片

docker-compose.yml
```yaml
services:
  random_local_asset_api:
    image: lambdayh/random_local_asset_api
    container_name: random_local_asset_api
    restart: unless-stopped
    environment:
      DOMAIN: "https://example.com/" # 返回的链接
    volumes:
      # 映射文件夹
      # assets文件夹下的文件夹为一个类别，递归遍历文件夹所有文件
      - "/srv/local_dir/:/assets/local_dir_in_docker"
    ports:
      - "8080:8080"
```
4. 
```
docker compose up -d
```

# 使用方法

`https://example.com/api/assets`

参数
1. category：必填。config中的category
2. type: `json` or `file`，如果是json，返回格式xxx，如果是file则直接返回文件链接
3. count: 默认1，文件数，当type为file时不支持
