# ImageFinder

Google Photos の写真を自然言語で検索するための MVP です。バックエンド（Go）、ワーカー（Go）、フロントエンド（React）を含みます。

## 構成

- `backend/`: API サーバとワーカー
- `frontend/`: React + TypeScript UI
- `infra/`: ローカル開発用の Postgres + pgvector
- `docs/`: API 仕様や運用メモ

## ローカル起動

### Docker でまとめて起動する

```bash
cd infra
docker compose up -d
```

`http://localhost:5173` を開くと UI が表示されます。API は `http://localhost:8080` で動作します。

### Google Photos と連携してテストする

1. `docs/oauth_setup.md` の手順で OAuth を設定します。
2. `backend/.env` を作成して認証情報を設定します。
3. 次のように環境変数を読み込んで起動します。

```bash
docker compose --env-file backend/.env -f infra/docker-compose.yml up -d
```

4. ブラウザでログインし、Picker から写真を選択して取り込みます。

### 手動で起動する

#### 1. DB 起動

```bash
cd infra
docker compose up -d
```

#### 2. 環境変数

```bash
cp backend/.env.example backend/.env
cp frontend/.env.example frontend/.env
```

`backend/.env` の `TOKEN_ENCRYPTION_KEY` は 32 バイトの base64 を設定してください。

例:

```bash
openssl rand -base64 32
```

#### 3. API サーバ

```bash
cd backend
go run ./cmd/api
```

#### 4. ワーカー

```bash
cd backend
go run ./cmd/worker
```

#### 5. フロントエンド

```bash
cd frontend
npm install
npm run dev
```

ブラウザで `http://localhost:5173` を開きます。

## モックモード

`OPENAI_MODE=mock` と `GEMINI_MODE=mock` が既定です。Google OAuth を未設定のままでも、開発環境では `/auth/google` が開発用ユーザーでログインし、ワーカーはモック写真を使ってインデックスを作成します。実際の Google Photos 連携を行う場合は `GOOGLE_OAUTH_CLIENT_ID` などを設定してください。実 API を使う場合は `OPENAI_MODE=api`、`GEMINI_MODE=api` を指定します（`real` も `api` と同じ扱いです）。

## Google Photos API について

- 2025 年以降は Picker API を使う構成にしています（`GOOGLE_PHOTOS_MODE=picker`）。

## データ保持方針

- 画像本体は保存せず、参照 URL のみ保持します。

## ドキュメント

- `docs/api.md`: API 仕様
- `docs/oauth_setup.md`: Google OAuth 設定手順
- `docs/operations.md`: 運用・監視メモ
- `docs/deploy.md`: デプロイ手順（概要）

## API

- `GET /healthz`
- `GET /auth/google`
- `GET /auth/callback`
- `POST /index/update`
- `GET /index/status`
- `POST /search`
- `GET /photos/:id`
