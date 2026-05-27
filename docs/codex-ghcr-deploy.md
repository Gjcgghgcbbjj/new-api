# Codex Docker 部署

这个分支的 Docker 镜像由 GitHub Actions 自动构建并推送到 GitHub Container Registry:

```text
ghcr.io/gjcgghgcbbjj/new-api:codex-latest
```

当前自动发布的是 `linux/amd64` 镜像，适合绝大多数 x86_64 VPS。

## VPS 快速部署

```bash
mkdir -p /opt/new-api-codex
cd /opt/new-api-codex
wget -O docker-compose.yml https://raw.githubusercontent.com/Gjcgghgcbbjj/new-api/codex/responses-chat-bridge/docker-compose.ghcr.yml
```

先修改 `docker-compose.yml` 里的三个值:

```text
CHANGE_ME_POSTGRES_PASSWORD
CHANGE_ME_REDIS_PASSWORD
CHANGE_ME_RANDOM_SESSION_SECRET
```

然后启动:

```bash
docker compose pull
docker compose up -d
```

访问:

```text
http://你的服务器IP:3000
```

## 更新

以后这个分支有新提交并且 GitHub Actions 构建成功后，在 VPS 执行:

```bash
cd /opt/new-api-codex
docker compose pull
docker compose up -d
```

## 如果拉取 GHCR 镜像失败

GitHub Packages 有时默认是 private。到仓库页面打开:

```text
Packages -> new-api -> Package settings -> Change visibility -> Public
```

如果你想保持 private，就在 VPS 登录 GHCR:

```bash
echo "你的GitHub PAT" | docker login ghcr.io -u Gjcgghgcbbjj --password-stdin
```
