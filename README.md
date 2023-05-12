一个返回随机本地资源的简陋API

# 部署方法

1. 创建个目录`folder`
2. 
```
cd folder
wget https://raw.githubusercontent.com/LambdaYH/random_local_asset_api/main/docker-compose.yml
wget https://raw.githubusercontent.com/LambdaYH/random_local_asset_api/main/config.yaml
```
3. 填写config.yaml与docker-compose.yml
假如本地文件夹为`/srv/local_dir/`，当访问`host/api/assets/category=local`返回此文件夹内的随机图片

则
config.yaml
```yaml
- folder: /local_dir_in_docker
  category: local
  suffixes: 
    - ".jpg"
    - ".png"
```
docker-compose.yml
```yaml
services:
  random_local_asset_api:
    image: lambdayh/random_local_asset_api
    container_name: random_local_asset_api
    restart: unless-stopped
    volumes:
      # 配置文件路径
      - "./config.yaml:/config.yaml"
      # 配置文件中填写的路径都要映射过去
      - "/srv/local_dir/:/local_dir_in_docker"
    ports:
      - "8080:8080"
```
4. 
```
docker compose up -d
```

# 使用方法

`https://your_host/api/assets`

参数
1. category：必填。config中的category
2. type: `json` or `file`，如果是json，返回格式xxx，如果是file则直接返回文件链接
3. count: 默认1，文件数，当type为file时不支持