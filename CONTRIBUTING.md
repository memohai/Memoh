# Contributing Guide

## Mise

You need to install mise first.

### Linux/macOS

```bash
curl https://mise.run | sh
```

Or use homebrew:

```bash
brew install mise
```

### Windows

```bash
winget install jdx.mise
```

## Initialize

Install toolchains and dependencies:

```bash
mise install
```

Setup project:

```bash
mise run setup
```

## Configure

Copy config.toml.example to config.toml and configure:

```bash
cp config.toml.example config.toml
```

## Development

Start development environment:

```bash
mise run dev
```

## More Commands

| Command | Description |
| ------- | ----------- |
| `mise run dev` | Start development environment |
| `mise run setup` | Setup development environment |
| `mise run db-up` | Initialize and Migrate Database |
| `mise run db-down` | Drop Database |
| `mise run swagger-generate` | Generate Swagger documentation |
| `mise run sqlc-generate` | Generate SQL code |
| `mise run pnpm-install` | Install dependencies |
| `mise run go-install` | Install Go dependencies |
| `mise run //agent:dev` | Start agent gateway development server |
| `mise run //cmd/agent:start` | Start main server |
| `mise run //packages/web:dev` | Start web development server |
| `mise run //packages/web:build` | Build web |
| `mise run //packages/web:start` | Start web preview |