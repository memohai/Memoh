# SQLite deployment

SQLite is available as a storage backend for single-node Memoh installs. It is a good fit for local demos, small self-hosted instances, and CI environments where running PostgreSQL is unnecessary.

Use PostgreSQL when you need a shared production database, higher write concurrency, external database operations, or a path toward multi-replica deployment.

## One-click install

Run the installer with `MEMOH_DATABASE_DRIVER=sqlite`:

```bash
curl -fsSL https://memoh.sh | MEMOH_DATABASE_DRIVER=sqlite sh
```

The equivalent flag form is:

```bash
curl -fsSL https://memoh.sh | sh -s -- --database-driver sqlite
```

The installer generates `config.toml` with:

```toml
[database]
driver = "sqlite"

[sqlite]
path = "/opt/memoh/data/memoh.db"
wal = true
busy_timeout_ms = 5000
```

It also uses `docker-compose.sqlite.yml`, which omits the PostgreSQL service and mounts the `memoh_data` volume into both the migration and server containers.

## Manual Docker install

```bash
git clone https://github.com/memohai/Memoh.git
cd Memoh
cp conf/app.docker.toml config.toml
```

Edit `config.toml`:

```toml
[database]
driver = "sqlite"

[sqlite]
path = "/opt/memoh/data/memoh.db"
wal = true
busy_timeout_ms = 5000
```

Then start the stack:

```bash
docker compose -f docker-compose.sqlite.yml --profile browser up -d
```

Add `--profile sparse` if you use the built-in memory provider in sparse mode.

## Development

SQLite has a separate development stack:

```bash
mise run dev:sqlite
```

Useful companion commands:

```bash
mise run dev:logs:sqlite
mise run dev:restart:sqlite -- server
mise run dev:down:sqlite
```

The development database path is `/opt/memoh/data/memoh.sqlite.db` inside the `memoh-dev-sqlite_memoh_data` Docker volume.

## Backups

For simple Docker deployments, stop Memoh before copying the database so the main database file and WAL file are consistent:

```bash
docker compose -f docker-compose.sqlite.yml down
docker run --rm \
  -v memoh_memoh_data:/data:ro \
  -v "$PWD":/backup \
  busybox tar czf /backup/memoh-sqlite-backup.tgz -C /data .
docker compose -f docker-compose.sqlite.yml --profile browser up -d
```

Restore by stopping the stack, unpacking the archive back into the `memoh_data` volume, and starting the stack again.
