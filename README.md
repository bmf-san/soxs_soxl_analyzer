# SOXL/SOXS 分析ツール 🐂🐻

**Go + 標準net/http + Plotly.js** で実装した SOXL / SOXS 売買支援ダッシュボード。

## 機能

- 価格・出来高チャート (SOXL / SOXS / SOXX / SMH 比較)
- テクニカル指標 (SMA / EMA / RSI / MACD / ボリンジャーバンド / ATR / HV)
- 売買シグナル生成 (4指標スコアリング)
- レバETFディケイ分析 (理論3倍 vs 実価格 / SOXL+SOXS両建てシミュ)
- バックテスト (MACD / RSI / MA_CROSS 戦略, Sharpe・PF・最大DD算出)
- 半導体関連ニュース取得

## セットアップ

```bash
git clone https://github.com/bmf-san/soxs_soxl_analyzer.git
cd soxs_soxl_analyzer
go mod tidy
go run ./cmd/server
```

ブラウザで http://localhost:8080 を開く。

## ビルド

```bash
go build -o bin/soxs-analyzer ./cmd/server
./bin/soxs-analyzer
```

## アーキテクチャ

```
soxs_soxl_analyzer/
├── cmd/server/main.go          # HTTPサーバーエントリ + ミドルウェア(ログ/レート制限)
├── internal/
│   ├── data/loader.go          # Yahoo Finance v8 chart API + キャッシュ
│   ├── indicators/indicators.go
│   ├── signals/signals.go
│   ├── decay/decay.go
│   ├── backtest/backtest.go
│   ├── plan/plan.go            # シグナル → 具体的トレードプラン
│   └── news/news.go
├── web/
│   ├── templates/index.html    # ダッシュボード
│   └── static/{app.js,style.css}
└── go.mod
```

## テスト

```bash
go test ./...
go test -cover ./...
```

各パッケージのカバレッジは概ね 80〜98%。

## API エンドポイント

| Method | Path | 説明 |
|--------|------|------|
| GET | `/` | ダッシュボードHTML |
| GET | `/api/prices?tickers=SOXL,SOXS,SOXX,SMH&period=1y` | OHLCV |
| GET | `/api/indicators?ticker=SOXL&period=1y` | 指標付き時系列 |
| GET | `/api/signal?ticker=SOXL&period=1y` | 最新シグナル |
| GET | `/api/decay?period=1y` | ディケイ分析 |
| GET | `/api/backtest?ticker=SOXL&strategy=MACD&period=1y&capital=10000&fee_bps=5` | バックテスト |
| GET | `/api/news?tickers=SOXL,SOXS,NVDA` | ニュース + センチメント |
| GET | `/api/plan?capital=10000&risk_pct=1.5&force=AUTO` | 具体的トレードプラン |

## ⚠️ 免責

教育目的。SOXL/SOXSは3倍レバレッジETFで極めてハイリスク。自己責任で。
