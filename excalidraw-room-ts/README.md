# excalidraw-room (TypeScript)

[excalidraw/excalidraw-room](https://github.com/excalidraw/excalidraw-room) を簡素化した TypeScript 実装です。

元の実装から不要な依存関係を取り除き、パッケージを最新化しました。

## 元の実装からの変更点

| 項目 | 変更前 | 変更後 |
|---|---|---|
| Node.js (Docker) | 12 | 22 |
| `socket.io` | 4.6.1 | 4.8.3 |
| TypeScript | 4.2.3 | 6.0 |
| tsconfig `target` | `es5` | `ES2022` |
| HTTP サーバー | `express` | `node:http`（標準ライブラリ） |
| ログ | `debug` パッケージ | `console.log` |
| 環境変数 | `dotenv` + `.env` ファイル | OS 環境変数から直接取得 |
| 開発サーバー | `ts-node-dev` | `tsx` |
| プロセス管理 | `pm2` | 不要（コンテナで管理） |
| Lint/Format | ESLint + Prettier 一式 | 削除 |
| Dockerfile | シングルステージ | マルチステージビルド |

## 動作要件

- Node.js 20 以上

## 起動

```bash
npm install
npm run build
npm start
```

開発時（ホットリロードあり）:

```bash
npm run dev
```

## 環境変数

| 変数 | デフォルト | 説明 |
|---|---|---|
| `PORT` | `3002` | リッスンポート |
| `CORS_ORIGIN` | `*`（全許可） | `Access-Control-Allow-Origin` に設定するオリジン |

## Docker

```bash
docker build -t excalidraw-room-ts .
docker run -p 3002:3002 excalidraw-room-ts
```

環境変数を渡す場合:

```bash
docker run -p 3002:3002 \
  -e CORS_ORIGIN=https://excalidraw.com \
  excalidraw-room-ts
```

## 参照実装

本実装は [excalidraw/excalidraw-room](https://github.com/excalidraw/excalidraw-room) をベースとしており、
MIT ライセンスのもと公開されているオリジナルの設計と実装に基づいています。
オリジナルの開発者・コントリビューターの皆様に感謝いたします。

## ライセンス

MIT
