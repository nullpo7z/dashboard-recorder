# Dashboard Recorder

## 概要

**Dashboard Recorder** は、高性能なコンテナ型ブラウザ録画サービスです。Go (Echo) と Playwright を使用して構築されており、ウェブページを指定されたフレームレートと解像度で動画ファイルとしてキャプチャします。また、ブラウザをリアルタイムで遠隔操作できるインタラクティブ機能も搭載しています。

## 機能

- **高画質録画**: タスクごとにフレームレート（1～15 FPS）を設定可能です。
- **インタラクティブモード**: OBSのブラウザソースのようにブラウザの操作が可能です。
- **カスタムCSS**: 録画対象ページに任意のCSSを注入し、不要な要素の非表示やスタイルの調整が可能です。

## 必要な環境

- **Docker**
- **Docker Compose**
- 推奨: メモリ 4GB以上 (ブラウザ実行のため)

## インストール方法

### 1. Docker Compose (推奨)

`compose.yml` を作成し、DockerHubからイメージを取得して起動します。

```yaml
services:
  app:
    image: nullpo7z/dashboard-recorder:latest
    container_name: dashboard_recorder
    restart: unless-stopped
    ports:
      - "8090:8080"
    environment:
      - TZ=Asia/Tokyo
      - LOG_LEVEL=info
      - JWT_SECRET=change_me_in_production
      - DATABASE_PATH=/app/data/app.db
    volumes:
      - ./backend_data:/app/data
      - ./backend_recordings:/app/recordings
    cap_drop:
      - ALL
    cap_add:
      - NET_BIND_SERVICE
    security_opt:
      - no-new-privileges:true
    user: "1000:1000"
```

```bash
# 起動
docker compose up -d
```

### 2. ソースコードからビルド (開発用)

リポジトリをクローンしてビルドします。

```bash
git clone https://github.com/nullpo7z/browser-recorder.git
cd browser-recorder
docker compose up -d --build
```

## 使用方法

### 1. ダッシュボードへのアクセス
ブラウザで `http://localhost:8090` にアクセスします。

### 2. ログイン
初期アカウント情報でログインします。
- **ユーザー名**: `admin`
- **パスワード**: `admin`

### 3. タスクの作成
Dashboardの「New Task」ボタンから録画タスクを作成します。
- **Task Name**: 管理用の名前
- **Target URL**: 録画したいウェブサイトのURL
- **Filename Prefix**: 録画ファイル名のテンプレート
- **FPS**: 1〜15の間で設定
- **Custom CSS**: 必要に応じてCSSを入力

### 4. 録画と操作
- **Start**: 録画を開始します。
- **Stop**: 録画を停止し、動画ファイルを保存します。
- **Interact**: ブラウザを直接操作します（クリック、文字入力可能）。
- **Setting**: タスクの設定を変更します。

## 注意事項

- **セキュリティ**: 初回ログイン後、**直ちにパスワードを変更してください**。また、本番環境で運用する場合は `compose.yml` の `JWT_SECRET` をランダムな文字列に変更してください。
- **パフォーマンス**: FPSの上限はサーバー負荷を考慮して **15 FPS** に制限されています。
- **データ永続化**: 録画データは `./backend_recordings`、データベースは `./backend_data` に保存されます。

## Web UIの機能

### Dashboard (ダッシュボード)
メインの管理画面です。
- **タスク管理**: 新規タスクの作成、編集、削除。
- **録画制御**: 各タスクの開始・停止。
- **インタラクト**: 「Interact」ボタンからブラウザを遠隔操作。

### Live Monitor (ライブモニター)
現在録画中のタスクのリアルタイムプレビューをグリッド表示します。動作状況を一目で確認できます。

### Archives (アーカイブ)
過去に録画された動画ファイルの一覧です。
- **ダウンロード**: ファイルをローカルに保存。
- **削除**: 不要なファイルを削除。

## ライセンス

[MIT License](LICENSE)
