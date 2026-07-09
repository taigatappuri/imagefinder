# ImageFinder

Google Photos の写真を自然言語で検索するための MVP です。バックエンド（Go）、ワーカー（Go）、フロントエンド（React）を含みます。

## 主な機能

- Google OAuth と Google Photos Picker API 連携
- Gemini による写真説明文の生成
- OpenAI Embeddings と pgvector による自然言語検索
- Cookie セッションによるユーザー単位のアクセス制御
- 外部 API を使わずに試せるモックモード

## 構成

- `backend/`: API サーバとワーカー
- `frontend/`: React + TypeScript UI
- `infra/`: ローカル開発用の Postgres + pgvector

## ローカル起動

```bash
cd infra
docker compose up -d
```

`http://localhost:5173` を開くと UI が表示されます。API は `http://localhost:8080` で動作します。

既定では `OPENAI_MODE=mock` と `GEMINI_MODE=mock` が使われます。Google OAuth を未設定のままでも、開発環境では `/auth/google` が開発用ユーザーでログインし、モック写真でインデックス作成と検索を試せます。

## 実 API を使う

1. Google Cloud で OAuth クライアントを作成し、リダイレクト URI に `http://localhost:8080/auth/callback` を設定します。
2. `backend/.env` を作成して認証情報を設定します。
3. 実 API を使う場合は `OPENAI_MODE=api` と `GEMINI_MODE=api` を指定します。
4. 次のように環境変数を読み込んで起動します。

```bash
docker compose --env-file backend/.env -f infra/docker-compose.yml up -d
```

ブラウザでログインし、Picker から写真を選択して取り込みます。

```bash
cp backend/.env.example backend/.env
cp frontend/.env.example frontend/.env
```

`backend/.env` の `TOKEN_ENCRYPTION_KEY` は 32 バイトの base64 を設定してください。Docker Compose で起動する場合は開発用の既定値が入りますが、本番環境では必ず変更してください。

例:

```bash
openssl rand -base64 32
```

## データ保持方針

- 画像本体は保存せず、参照 URL のみ保持します。
- OAuth トークンはローカル鍵または AWS KMS で暗号化して保存します。
