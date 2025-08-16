# CLAUDE.md

このファイルはClaude Code（claude.ai/code）がこのリポジトリのコードを扱う際のガイダンスを提供する。

## 回答方針

- ネットワークプログラミングの専門家として技術的に正確な回答を提供する
- 穏当で適切な表現を使用し、過度な感嘆符や媚びるような表現は避ける
- 事実に基づいた回答のみを行い、推測や類推による回答は行わない

## プロジェクト概要

SynpackはGoで書かれた軽量なTCPハートビートツールで、TCP SYNパケットを送信してネットワーク接続をテストし、往復時間を測定する。rawソケットを使用するためroot権限が必要。

## よく使うコマンド

### ビルド
```bash
# 静的バイナリ用にCGOを無効にしてビルド
CGO_ENABLED=0 go build -o synpack

# 通常のビルド
go build -o synpack
```

### テスト
```bash
# 全テストを実行
go test

# 詳細出力付きでテストを実行
go test -v

# 特定のテストを実行
go test -run TestHasDockerInterfaceName
```

### フォーマット・Lint
```bash
# コードをフォーマット
go fmt

# 潜在的な問題をチェック
go vet
```

### 実行
```bash
# ビルド後に実行（rawソケットアクセスのためsudoが必要）
sudo ./synpack -h github.com -p 80 -c 3

# go runで直接実行
sudo go run synpack.go -h github.com -p 80 -c 3

# 複数のターゲットをテスト
sudo go run synpack.go -h google.com -p 443 -c 5
sudo go run synpack.go -h 8.8.8.8 -p 53 -c 3
```

## コードアーキテクチャ

### シングルパッケージ構成
- **synpack.go**: TCP SYNパケット実装を含むメインアプリケーション
- **synpack_test.go**: 主要機能のユニットテスト

### 主要コンポーネント

#### Network Layer (synpack.go)
- **Raw Socket Management**: `golang.org/x/sys/unix`を使用したraw TCPソケットの作成と管理
- **TCP Packet Construction**: 適切なチェックサムを含むTCPヘッダーの手動作成
- **Network Interface Detection**: Docker/コンテナインターフェースを除外してアクティブなネットワークインターフェースを自動検出
- **Port Management**: システム提供の利用可能ポートを使用した動的ポート割り当て

#### 主要関数
- `createTcpHeader()`: 適切なヘッダーとチェックサムを持つTCP SYNパケットを構築
- `parsePacket()`: 受信パケットを解析してSYN-ACK応答を識別
- `getLocalInterface()`: アクティブなネットワークインターフェースとIPアドレスを検出
- `generateAvailablePort()`: システムから利用可能ポートを取得
- `hasDockerInterfaceName()`: コンテナ関連のネットワークインターフェースを除外

#### パケットフロー
1. TCPプロトコルでrawソケットを作成
2. ランダムシーケンス番号を生成
3. 適切なヘッダーを持つTCP SYNパケットを構築
4. 対象ホスト/ポートにパケット送信
5. 一致するシーケンスのSYN-ACK応答を待機
6. 往復時間を計算して統計を表示

### 依存関係
- **golang.org/x/sys/unix**: rawソケット操作のための低レベルシステムコール
- **Standard library**: ネットワーキングとパケット操作のためのnet、crypto/rand、encoding/binary

### セキュリティ考慮事項
- rawソケットアクセスのためroot権限が必要
- 無関係なトラフィックの処理を避けるため適切なパケット検証を実装
- シーケンス番号に暗号学的に安全な乱数生成を使用

### 動作確認済み環境
- Ubuntu 20.04, 22.04

### macOS環境での制限
- macOSのSIP(System Integrity Protection)によりセキュリティの制限がある。
- macOSではraw socketを利用する際の制限がLinuxより厳しいと考えられる。
- sudoを付与して実行するだけでなく「プライバシーとセキュリティ」の設定変更が必要と思われる。
- `golang.org/x/sys/unix` を利用する際にmacOS固有の指定方法を調査する必要あり。
    - 送信パケットを作成する際にTCPヘッダーだけでなくIPヘッダーも作成する必要あり。
    - パケット受信時に `unix.IPPROTO_TCP` ではなく `unix.IPPROTO_RAW` を利用する。
        - `IP_HDRINCL` オプションを指定する。
    - `unix.Sendto`は機能するが、`unix.Recvfrom`が制限される模様。

### 参考資料
- https://sock-raw.org/papers/sock_raw 
- https://stackoverflow.com/questions/2438471/raw-socket-sendto-failure-in-os-x
- https://news.ycombinator.com/item?id=41537426
- https://stackoverflow.com/questions/79382235/proper-way-to-obtain-dynamic-source-port-for-raw-socket-ip-hdrincl-on-macos-fo
