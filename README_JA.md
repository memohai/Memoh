<div align="right">
  <span>[<a href="./README.md">English</a>]<span>
  </span>[<a href="./README_CN.md">简体中文</a>]</span>
  </span>[<a href="./README_JA.md">日本語</a>]</span>
</div>

<div align="center">
  <img src="./assets/logo.png" alt="Memoh" height="80">
  <h1>Memoh</h1>
  <p>オープンソースのマルチ Agent プラットフォーム<br>
  すべての Agent に専用のコンピューター、Desktop、ネットワーク、長期記憶を</p>
  <div align="center">
    <img src="https://img.shields.io/github/package-json/v/memohai/Memoh" alt="Version" />
    <img src="https://img.shields.io/github/stars/memohai/Memoh?style=social" alt="Stars" />
    <img src="https://img.shields.io/github/forks/memohai/Memoh?style=social" alt="Forks" />
    <a href="https://deepwiki.com/memohai/Memoh">
      <img src="https://deepwiki.com/badge.svg" alt="DeepWiki" />
    </a>
    <a href="https://t.me/memohai">
      <img src="https://img.shields.io/badge/Telegram-Group-26A5E4?logo=telegram&logoColor=white" alt="Telegram" />
    </a>
  </div>
  <h3>
    <a href="https://memoh.ai/desktop">Memoh Desktop をダウンロード</a> · <a href="#server-にデプロイ">Server にデプロイ</a> · <a href="https://memoh.ai">Website</a> · <a href="https://x.com/memoh_ai">X</a> · <a href="https://docs.memoh.ai">Docs</a>
  </h3>
  <img src="./assets/hero.png" alt="Memoh" width="1000">
</div>

> [!TIP]
> Memoh の SaaS サービスは近日公開予定です。[memoh.ai/waitlist](https://memoh.ai/waitlist) から waitlist に参加できます。

## Memoh とは？

Memoh は、1 台のマシン上で複数の AI Agent を動かすためのプラットフォームです。各 Agent には専用の Container、ファイルシステム、Desktop、Browser、ネットワーク、長期記憶が割り当てられます。まるで Agent 専用のコンピューターを持たせるような感覚です。

Telegram、Discord、Lark、WeChat、Web UI などから Agent と会話できます。Agent は文脈を記憶し、Browser を操作し、MCP ツールを呼び出し、スケジュールされたタスクも実行できます。

自分用に 1 つ動かすことも、チームメンバーごとに 1 つずつ割り当てることも、1 台のマシン上で専用 Agent の fleet をまとめて動かすこともできます。

## Quick Start

### Desktop

[Memoh Desktop をダウンロード](https://memoh.ai/desktop)

### Server にデプロイ

```bash
curl -fsSL https://memoh.sh | sh
```

<details>
<summary><strong>その他のデプロイ方法</strong></summary>

手動でデプロイする場合:

```bash
git clone --depth 1 https://github.com/memohai/Memoh.git
cd Memoh
cp conf/app.docker.toml config.toml
# config.toml を編集
docker compose up -d
```

> **イメージの pull が遅い場合は CN mirror を利用できます:**
> ```bash
> curl -fsSL https://memoh.sh | USE_CN_MIRROR=true sh
> ```
>
> インストーラー全体を `sudo` で実行しないでください。Docker に権限が必要な場合、インストーラー内部で `sudo docker` を使用します。

カスタム設定や本番環境での構成については [DEPLOYMENT.md](DEPLOYMENT.md) を参照してください。

</details>

## Memoh を選ぶ理由

- **すべての Agent に専用コンピューター**: 専用のファイルシステム、ネットワーク、Desktop、Browser を備えた隔離 Container。
- **Multi-user, multi-bot**: 自分用に 1 つ、家族やチームメンバーごとに 1 つ、または 1 台のマシンで複数の Bot をまとめて運用できます。
- **軽量**: Edge device でも動作します。推論は cloud に任せ、データは local に残せます。

## Features

### Core

- **Multi-bot & multi-user**: 複数の Bot が、個別チャット、グループチャット、Bot 同士の会話に対応します。Cross-platform identity binding も利用できます。
- **Containerized Workspace**: 各 Bot は専用の Container で動作し、ファイルシステム、ネットワーク、Tool、Desktop を持ちます。
- **Built-in memory**: セッションやプラットフォームをまたいだ長期記憶を標準搭載。[Mem0](https://mem0.ai) や OpenViking も利用できます。
- **10+ channels**: Telegram、Discord、Lark、WeChat、QQ、Email などに対応しています。

### Agent Capabilities

- **MCP**: 外部 Tool server に接続できます。各 Bot が自分の接続を管理します。
- **Browser Use**: Container 内の Browser を操作できます。
- **Computer Use**: GUI が必要な作業のために Container の Desktop を操作できます。
- **Skills & Supermarket**: モジュール化された Skill、Supermarket からの curated template インストール、sub-agent への委譲に対応します。
- **Automation**: スケジュールタスクと周期的な heartbeat を実行できます。

## Memory

Memoh には完全に self-hosted できる memory engine が含まれています。各 Bot は、セッション、日付、プラットフォームをまたいで、あなたが伝えた内容を記憶できます。

[**Mem0**](https://mem0.ai) や **OpenViking** も差し替え可能な選択肢として利用できます。詳しくは [documentation](https://docs.memoh.ai/memory-providers/) を参照してください。

## Sub-projects

- [**Twilight AI**](https://github.com/memohai/twilight-ai) — Go 向けの軽量で idiomatic な AI SDK。[Vercel AI SDK](https://sdk.vercel.ai/) に着想を得ており、Provider 非依存で、streaming、tool calling、MCP、embeddings を first-class に扱えます。

## Project Status

![License](https://img.shields.io/github/license/memohai/Memoh) ![Last Commit](https://img.shields.io/github/last-commit/memohai/Memoh) ![Commit Activity](https://img.shields.io/github/commit-activity/m/memohai/Memoh) ![Issues](https://img.shields.io/github/issues/memohai/Memoh) ![Pull Requests](https://img.shields.io/github/issues-pr/memohai/Memoh)

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=memohai/Memoh&type=date&legend=top-left)](https://www.star-history.com/#memohai/Memoh&type=date&legend=top-left)

## Contributors

<a href="https://github.com/memohai/Memoh/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=memohai/Memoh" />
</a>

## Community

- 🌐 [**Website**](https://memoh.ai)
- 📚 [**Documentation**](https://docs.memoh.ai)
- 💬 [**Telegram Group**](https://t.me/memohai)
- 🛒 [**Supermarket**](https://github.com/memohai/supermarket)
- 🤝 [**Cooperation**](mailto:business@memoh.net) — business@memoh.net

---

**LICENSE**: AGPLv3

Made with ❤️ by MemohAI Team,

Copyright (C) 2026 MemohAI (memoh.ai). All rights reserved.
