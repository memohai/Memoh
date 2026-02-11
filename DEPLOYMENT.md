# Memoh Docker Deployment Guide

Deploy Memoh AI Agent System with Docker Compose in one command.

## Quick Start

### 1. Clone the Repository
```bash
git clone https://github.com/memohai/Memoh.git
cd Memoh
```

### 2. One-Click Deployment
```bash
./deploy.sh
```

The script will automatically:
- Check Docker and Docker Compose installation
- Create `.env` configuration file (if not exists)
- Generate random JWT secret
- Create `config.toml` configuration file
- Build MCP image
- Start all services

### 3. Access the Application
- Web UI: http://localhost
- API Service: http://localhost:8080
- Agent Gateway: http://localhost:8081

Default admin credentials:
- Username: `admin`
- Password: `admin123` (change in `.env`)

## Manual Deployment

If you prefer not to use the automated script:

```bash
# 1. Create configuration files
cp .env.example .env
cp config.docker.toml config.toml

# 2. Edit configuration (Important!)
nano .env

# 3. Generate JWT secret
openssl rand -base64 32

# 4. Build MCP image
docker build -f cmd/mcp/Dockerfile -t memoh-mcp:latest .

# 5. Start services
docker compose up -d

# 6. View logs
docker compose logs -f
```

## Architecture

This deployment uses the host's Docker daemon to manage Bot containers:

```
Host Docker
├── memoh-postgres (PostgreSQL)
├── memoh-qdrant (Qdrant)
├── memoh-server (Main Service) ← Manages Bot containers via /var/run/docker.sock
├── memoh-agent (Agent Gateway)
├── memoh-web (Web Frontend)
└── memoh-bot-* (Bot containers, dynamically created by main service)
```

Advantages:
- ✅ Lightweight, no additional Docker daemon needed
- ✅ Better performance, uses host container runtime directly
- ✅ Easier to manage and debug
- ✅ Lower resource consumption

## Common Commands

### Using Make (Recommended)
```bash
make help       # Show all commands
make deploy     # One-click deployment
make logs       # View logs
make restart    # Restart services
make ps         # View status
make backup     # Backup data
make bots       # View Bot containers
```

### Using Docker Compose
```bash
docker compose up -d        # Start services
docker compose down         # Stop services
docker compose logs -f      # View logs
docker compose ps           # View status
docker compose restart      # Restart services
```

## Configuration

### Environment Variables (.env)

Key configuration items:

```bash
# PostgreSQL password (must change)
POSTGRES_PASSWORD=your_secure_password

# JWT secret (must change)
JWT_SECRET=your_random_jwt_secret

# Admin account
ADMIN_USERNAME=admin
ADMIN_PASSWORD=your_admin_password
ADMIN_EMAIL=admin@yourdomain.com
```

### Application Configuration (config.toml)

Main configuration items:

```toml
[postgres]
host = "postgres"
password = "your_secure_password"  # Must match POSTGRES_PASSWORD in .env

[containerd]
socket_path = "unix:///var/run/docker.sock"  # Use host Docker

[qdrant]
base_url = "http://qdrant:6334"
```

## Service Overview

| Service | Container Name | Ports | Description |
|---------|---------------|-------|-------------|
| postgres | memoh-postgres | - | PostgreSQL database (internal only) |
| qdrant | memoh-qdrant | - | Qdrant vector database (internal only) |
| docker-cli | memoh-docker-cli | - | Docker CLI (uses host Docker) |
| server | memoh-server | 8080 | Main service (Go) |
| agent | memoh-agent | 8081 | Agent Gateway (Bun) |
| web | memoh-web | 80 | Web frontend (Nginx) |

## Data Persistence

Data is stored in Docker volumes:

```bash
# View volumes
docker volume ls | grep memoh

# Backup database
docker compose exec postgres pg_dump -U memoh memoh > backup.sql
```

### Bot Container Management

Bot containers are dynamically created by the main service and run directly on the host:

```bash
# View all Bot containers
make bots
# or
docker ps -a | grep memoh-bot

# View Bot logs
docker logs <bot-container-id>

# Enter Bot container
docker exec -it <bot-container-id> sh

# Stop Bot container
docker stop <bot-container-id>
```

## Backup and Restore

### Backup
```bash
# Create backup directory
mkdir -p backups

# Backup database
docker compose exec postgres pg_dump -U memoh memoh > backups/postgres_$(date +%Y%m%d).sql

# Backup Bot data
docker run --rm -v memoh_memoh_bot_data:/data -v $(pwd)/backups:/backup alpine \
  tar czf /backup/bot_data_$(date +%Y%m%d).tar.gz -C /data .

# Backup configuration files
tar czf backups/config_$(date +%Y%m%d).tar.gz config.toml .env
```

### Restore
```bash
# Restore database
docker compose exec -T postgres psql -U memoh memoh < backups/postgres_20240101.sql

# Restore Bot data
docker run --rm -v memoh_memoh_bot_data:/data -v $(pwd)/backups:/backup alpine \
  tar xzf /backup/bot_data_20240101.tar.gz -C /data
```

## Troubleshooting

### Services Won't Start
```bash
# View detailed logs
docker compose logs server

# Check configuration
docker compose config

# Rebuild
docker compose build --no-cache
docker compose up -d
```

### Database Connection Failed
```bash
# Check if database is ready
docker compose exec postgres pg_isready -U memoh

# Test connection
docker compose exec postgres psql -U memoh -d memoh

# View database logs
docker compose logs postgres
```

### Port Conflicts
```bash
# Check port usage
sudo netstat -tlnp | grep :8080
sudo netstat -tlnp | grep :80

# Modify port mapping in docker-compose.yml
# Example: change "80:80" to "8000:80"
```

### Docker Socket Permission Issues
```bash
# Add user to docker group
sudo usermod -aG docker $USER
newgrp docker

# Check permissions
ls -la /var/run/docker.sock
```

## Production Deployment

### 1. Use HTTPS

Create `docker-compose.override.yml`:
```yaml
services:
  web:
    ports:
      - "443:443"
    volumes:
      - ./ssl:/etc/nginx/ssl:ro
      - ./nginx-https.conf:/etc/nginx/conf.d/default.conf:ro
```

Create `nginx-https.conf`:
```nginx
server {
    listen 80;
    server_name your-domain.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name your-domain.com;
    
    ssl_certificate /etc/nginx/ssl/cert.pem;
    ssl_certificate_key /etc/nginx/ssl/key.pem;
    
    # SSL configuration
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;
    
    # Other configurations same as nginx.conf
    # ...
}
```

### 2. Resource Limits

Edit `docker-compose.yml` to add resource limits:
```yaml
services:
  server:
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
        reservations:
          cpus: '1'
          memory: 1G
```

### 3. Security Recommendations

Production environment recommendations:
- Use separate `.env` file
- Change all default passwords
- Use strong JWT secret
- Configure firewall rules
- Use HTTPS
- Regular data backups
- Limit containerd socket access permissions
- Run services as non-root user
- Configure log rotation

## Performance Optimization

### PostgreSQL Optimization
Create `postgres-custom.conf`:
```
shared_buffers = 2GB
effective_cache_size = 6GB
maintenance_work_mem = 512MB
checkpoint_completion_target = 0.9
wal_buffers = 16MB
```

Mount in `docker-compose.yml`:
```yaml
postgres:
  volumes:
    - ./postgres-custom.conf:/etc/postgresql/postgresql.conf:ro
  command: postgres -c config_file=/etc/postgresql/postgresql.conf
```

### Network Optimization
```yaml
networks:
  memoh-network:
    driver: bridge
    driver_opts:
      com.docker.network.driver.mtu: 1500
```

## Update Application

```bash
# Pull latest code
git pull

# Rebuild and restart
docker compose up -d --build

# Or use Make
make update
```

## Complete Uninstall

```bash
# Stop and remove all containers
docker compose down

# Remove data volumes (Warning! This deletes all data)
docker compose down -v

# Remove images
docker rmi memoh-mcp:latest
docker rmi $(docker images | grep memoh | awk '{print $3}')
```

## Security Considerations

⚠️ Important Security Notes:

1. **Docker Socket Access**: The main service container has access to the host Docker socket, which means the application can manage other containers on the host. Only run in trusted environments.
2. **Change Default Passwords**: Must change all default passwords in `.env`
3. **Strong JWT Secret**: Use a strong random JWT secret
4. **Firewall**: Configure firewall to only open necessary ports
5. **HTTPS**: Use HTTPS in production
6. **Regular Backups**: Regularly backup data
7. **Updates**: Regularly update images and dependencies

## Get Help

- Detailed Documentation: [DOCKER_DEPLOYMENT_CN.md](DOCKER_DEPLOYMENT_CN.md) (Chinese)
- GitHub Issues: https://github.com/memohai/Memoh/issues
- Telegram Group: https://t.me/memohai
- Email: business@memoh.net

---

**That's it! Deploy Memoh in minutes!** 
