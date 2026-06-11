# excalidraw-room (Go)

[excalidraw/excalidraw-room](https://github.com/excalidraw/excalidraw-room) の Go 実装です。

Engine.IO v4 + Socket.IO v4 のプロトコルを最小限に実装し、Excalidraw のリアルタイム共同編集に必要なイベントのみを提供します。外部依存は [`gorilla/websocket`](https://github.com/gorilla/websocket) のみです。

## 動作要件

- Go 1.21 以上

## ビルドと起動

```bash
cd excalidraw-room
go build -o server .
./server
```

デフォルトでポート `3002` で起動します。

## 環境変数

| 変数 | デフォルト | 説明 |
|---|---|---|
| `PORT` | `3002` | リッスンポート |
| `CORS_ORIGIN` | `*`（全許可） | `Access-Control-Allow-Origin` に設定するオリジン |

## Docker

```bash
docker build -t excalidraw-room .
docker run -p 3002:3002 excalidraw-room
```

環境変数を渡す場合:

```bash
docker run -p 3002:3002 \
  -e PORT=3002 \
  -e CORS_ORIGIN=https://excalidraw.com \
  excalidraw-room
```

## Excalidraw からの接続

Excalidraw の `VITE_APP_WS_SERVER_URL`（または `REACT_APP_WS_SERVER_URL`）にこのサーバーの URL を指定してください。

```
VITE_APP_WS_SERVER_URL=ws://localhost:3002
```

WebSocket トランスポートのみサポートしています（HTTP ポーリングは非対応）。

## 実装範囲

| イベント | 方向 | 説明 |
|---|---|---|
| `init-room` | Server → Client | 接続確立時に送信 |
| `join-room` | Client → Server | ルームへの参加 |
| `first-in-room` | Server → Client | ルームの最初の参加者に送信 |
| `new-user` | Server → Client | 既存メンバーへ新規参加者を通知 |
| `room-user-change` | Server → Client | ルームのメンバー一覧更新 |
| `server-broadcast` | Client → Server | 暗号化描画データをルーム全員に中継 |
| `server-volatile-broadcast` | Client → Server | ロス許容の中継（カーソル位置など） |
| `client-broadcast` | Server → Client | 中継された描画データの受信 |
| `user-follow` | Client → Server | ユーザーのフォロー／アンフォロー |
| `user-follow-room-change` | Server → Client | フォロワー一覧更新 |
| `broadcast-unfollow` | Server → Client | フォロー解除通知 |

## 参照実装

本実装は [excalidraw/excalidraw-room](https://github.com/excalidraw/excalidraw-room) を参考にしています。
オリジナルは Node.js + Socket.IO で実装されており、MIT ライセンスのもと公開されています。

プロトコルの設計および各イベントの仕様は excalidraw-room の実装に準拠しています。
オリジナルの開発者・コントリビューターの皆様に感謝いたします。

## ライセンス

MIT
