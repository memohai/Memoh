# SQLite 部署

Memoh 可以用 SQLite 作为存储后端。它适合本地演示、小型自托管实例和 CI 环境，不需要额外跑 PostgreSQL。

如果你需要共享生产数据库、更高写入并发、成熟的数据库运维，或者以后可能多副本部署，还是建议用 PostgreSQL。

## 一键安装

用 `MEMOH_DATABASE_DRIVER=sqlite` 运行安装脚本：

```bash
curl -fsSL https://memoh.sh | MEMOH_DATABASE_DRIVER=sqlite sh
```

也可以用参数：

```bash
curl -fsSL https://memoh.sh | sh -s -- --database-driver sqlite
```

脚本会生成这样的 `config.toml`：

```toml
[database]
driver = "sqlite"

[sqlite]
path = "/opt/memoh/data/memoh.db"
wal = true
busy_timeout_ms = 5000
```

SQLite 模式会使用 `docker-compose.sqlite.yml`，不启动 PostgreSQL。迁移容器和服务容器都会挂载同一个 `memoh_data` volume，所以迁移写入的数据库文件会被服务继续使用。

## 手动 Docker 安装

```bash
git clone https://github.com/memohai/Memoh.git
cd Memoh
cp conf/app.docker.toml config.toml
```

编辑 `config.toml`：

```toml
[database]
driver = "sqlite"

[sqlite]
path = "/opt/memoh/data/memoh.db"
wal = true
busy_timeout_ms = 5000
```

启动：

```bash
docker compose -f docker-compose.sqlite.yml --profile qdrant --profile browser up -d
```

如果你要用内置记忆的 sparse 模式，再加 `--profile sparse`。

## 开发环境

SQLite 有单独的开发栈：

```bash
mise run dev:sqlite
```

常用命令：

```bash
mise run dev:logs:sqlite
mise run dev:restart:sqlite -- server
mise run dev:down:sqlite
```

开发环境里的数据库路径是 `/opt/memoh/data/memoh.sqlite.db`，在 `memoh-dev-sqlite_memoh_data` Docker volume 中。

## 备份

简单部署下，先停服务再备份，避免主数据库文件和 WAL 文件不一致：

```bash
docker compose -f docker-compose.sqlite.yml down
docker run --rm \
  -v memoh_memoh_data:/data:ro \
  -v "$PWD":/backup \
  busybox tar czf /backup/memoh-sqlite-backup.tgz -C /data .
docker compose -f docker-compose.sqlite.yml --profile qdrant --profile browser up -d
```

恢复时先停服务，把压缩包解回 `memoh_data` volume，再启动。
