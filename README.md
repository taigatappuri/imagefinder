# ImageFinder

Google Photos の写真を自然言語で検索するための MVP です。バックエンド（Go）、ワーカー（Go）、フロントエンド（React）を含みます。

## 構成

- `backend/`: API サーバとワーカー
- `frontend/`: React + TypeScript UI
- `infra/`: ローカル開発用の Postgres + pgvector

## ローカル起動

### Docker でまとめて起動する

```bash
cd infra
docker compose up -d
```

`http://localhost:5173` を開くと UI が表示されます。API は `http://localhost:8080` で動作します。

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

`OPENAI_MODE=mock` と `GEMINI_MODE=mock` が既定です。Google OAuth を未設定のままでも、開発環境では `/auth/google` が開発用ユーザーでログインし、ワーカーはモック写真を使ってインデックスを作成します。実際の Google Photos 連携を行う場合は `GOOGLE_OAUTH_CLIENT_ID` などを設定してください。

## API

- `GET /healthz`
- `GET /auth/google`
- `GET /auth/callback`
- `POST /index/update`
- `GET /index/status`
- `POST /search`
- `GET /photos/:id`
