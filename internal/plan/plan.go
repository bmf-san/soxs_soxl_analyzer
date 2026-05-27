package plan

// Package plan turns indicator + signal output into a concrete trading plan
// (action, entry, stop-loss, take-profits, position size).

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/bmf-san/soxs_soxl_analyzer/internal/data"
	"github.com/bmf-san/soxs_soxl_analyzer/internal/indicators"
	"github.com/bmf-san/soxs_soxl_analyzer/internal/signals"
)

// Plan represents a concrete trade plan.
type Plan struct {
	Action          string   `json:"action"`       // "BUY_SOXL" | "BUY_SOXS" | "HOLD"
	ActionLabel     string   `json:"action_label"` // 日本語ラベル
	Ticker          string   `json:"ticker"`       // 推奨売買銘柄
	Confidence      string   `json:"confidence"`   // 強 / 中 / 弱
	SignalScore     float64  `json:"signal_score"` // -1.0 〜 +1.0
	CurrentPrice    float64  `json:"current_price"`
	EntryPrice      float64  `json:"entry_price"`
	StopLoss        float64  `json:"stop_loss"`
	TakeProfit1     float64  `json:"take_profit_1"` // ATR×2
	TakeProfit2     float64  `json:"take_profit_2"` // ATR×3
	ATR             float64  `json:"atr"`
	HV20            float64  `json:"hv20"`
	RSI14           float64  `json:"rsi14"`
	PositionUSD     float64  `json:"position_usd"`
	PositionPct     float64  `json:"position_pct"` // 総資産対する%
	Shares          int      `json:"shares"`
	RiskUSD         float64  `json:"risk_usd"` // 損切り時の最大損失
	RiskPctOfTotal  float64  `json:"risk_pct_of_total"`
	RewardUSD       float64  `json:"reward_usd"` // TP1までの利益
	RiskRewardRatio float64  `json:"risk_reward_ratio"`
	TimeStopDays    int      `json:"time_stop_days"`
	Reasoning       []string `json:"reasoning"`
	Warnings        []string `json:"warnings"`
}

// Build computes a plan.
// loadSeries is the function used to fetch OHLCV. Tests may override this.
var loadSeries = data.LoadSeries

// capital: 総投資余力 (USD), riskPct: 1トレードあたりの許容損失 (% of capital, default 1.5)
// force: "" / "AUTO" → シグナルで自動決定。"SOXL" or "SOXS" → それを強制。
func Build(capital, riskPct float64, force string) (*Plan, error) {
	if riskPct <= 0 {
		riskPct = 1.5
	}
	force = strings.ToUpper(strings.TrimSpace(force))
	// We always analyze SOXL price action as the proxy for direction.
	// If signal is bearish, we recommend SOXS instead (also 3x but inverse).
	s, err := loadSeries("SOXL", "6mo")
	if err != nil {
		return nil, err
	}
	closes := s.CloseSlice()
	high := make([]float64, len(s.Bars))
	low := make([]float64, len(s.Bars))
	for i, b := range s.Bars {
		high[i] = b.High
		low[i] = b.Low
	}
	ind := indicators.AttachAll(high, low, closes)
	sig := signals.Generate(closes, ind)

	currentSOXL := closes[len(closes)-1]
	atrSOXL := lastNonZero(ind.ATR14)
	hv := lastNonZero(ind.HV20)
	rsi := ind.RSI14[len(ind.RSI14)-1]

	plan := &Plan{
		SignalScore:  sig.Score,
		CurrentPrice: currentSOXL, // default; overwritten for SOXS
		ATR:          round2(atrSOXL),
		HV20:         round2(hv),
		RSI14:        round2(rsi),
		Reasoning:    []string{},
		Warnings:     []string{},
	}

	// ── Decide action ──────────────────────────────────────
	confidenceOf := func(abs float64) string {
		switch {
		case abs >= 0.5:
			return "強"
		case abs >= 0.2:
			return "弱"
		default:
			return "—"
		}
	}

	var chosen string // "SOXL" / "SOXS" / "HOLD"
	switch force {
	case "SOXL":
		chosen = "SOXL"
		if sig.Score < 0.2 {
			plan.Warnings = append(plan.Warnings, "ユーザー強制で SOXL プラン表示中。シグナルは中立〜弱気なので逆行リスクあり。")
		}
	case "SOXS":
		chosen = "SOXS"
		if sig.Score > -0.2 {
			plan.Warnings = append(plan.Warnings, "ユーザー強制で SOXS プラン表示中。シグナルは中立〜強気なので逆行リスクあり。")
		}
	default:
		switch {
		case sig.Score >= 0.2:
			chosen = "SOXL"
		case sig.Score <= -0.2:
			chosen = "SOXS"
		default:
			chosen = "HOLD"
		}
	}

	switch chosen {
	case "SOXL":
		plan.Action = "BUY_SOXL"
		conf := confidenceOf(math.Abs(sig.Score))
		if force == "SOXL" && sig.Score < 0.2 {
			conf = "逆行"
		}
		plan.Confidence = conf
		plan.ActionLabel = "🐂 SOXL 買い (" + conf + ")"
		plan.Ticker = "SOXL"
		plan.CurrentPrice = currentSOXL
	case "SOXS":
		plan.Action = "BUY_SOXS"
		conf := confidenceOf(math.Abs(sig.Score))
		if force == "SOXS" && sig.Score > -0.2 {
			conf = "逆行"
		}
		plan.Confidence = conf
		plan.ActionLabel = "🐻 SOXS 買い (" + conf + ") ← 下落に賭ける"
		plan.Ticker = "SOXS"
	default: // HOLD
		plan.Action = "HOLD"
		plan.ActionLabel = "⚪️ 様子見（エントリーしない）"
		plan.Confidence = "—"
		plan.Ticker = "—"
		plan.EntryPrice = round2(currentSOXL)
		plan.Reasoning = append(plan.Reasoning,
			"シグナルスコア "+fmtf(sig.Score)+" で中立圏。明確な方向感が出るまで現金保有が最善。",
			"無理にエントリーすると手数料とディケイで損する確率が高い。",
			"「強制SOXL」「強制SOXS」ボタンで両シナリオのプランを見ることも可能。",
		)
		return plan, nil
	}

	// ── Price targets ─────────────────────────────────────
	// SOXS用にはSOXS自身の最新価格・ATRが必要
	if plan.Action == "BUY_SOXS" {
		soxs, sErr := loadSeries("SOXS", "6mo")
		if sErr != nil || len(soxs.Bars) == 0 {
			return nil, fmt.Errorf("SOXS価格取得失敗: %v", sErr)
		}
		sc := soxs.CloseSlice()
		sh := make([]float64, len(soxs.Bars))
		sl := make([]float64, len(soxs.Bars))
		for i, b := range soxs.Bars {
			sh[i] = b.High
			sl[i] = b.Low
		}
		soxsATR := lastNonZero(indicators.ATR(sh, sl, sc, 14))
		plan.CurrentPrice = sc[len(sc)-1]
		plan.ATR = round2(soxsATR)
	}
	if plan.CurrentPrice <= 0 || plan.ATR <= 0 {
		return nil, fmt.Errorf("価格またはATRが取得できませんでした (price=%.2f, atr=%.2f)", plan.CurrentPrice, plan.ATR)
	}

	atr := plan.ATR
	price := plan.CurrentPrice
	plan.EntryPrice = round2(price)
	plan.StopLoss = round2(price - 1.5*atr)
	plan.TakeProfit1 = round2(price + 2.0*atr)
	plan.TakeProfit2 = round2(price + 3.0*atr)

	// ── Position sizing ────────────────────────────────────
	// HVに応じてサイズ係数を変える: ボラ高いほど小さく
	hvFactor := 1.0
	switch {
	case hv >= 100:
		hvFactor = 0.4
		plan.Warnings = append(plan.Warnings, "年率ボラが100%超え。極端に高い水準なのでサイズ控えめ。")
	case hv >= 80:
		hvFactor = 0.6
	case hv >= 50:
		hvFactor = 0.8
	}
	// 信頼度（シグナル絶対値）でも係数調整
	confFactor := math.Min(1.0, math.Abs(sig.Score)+0.3)

	// 1) リスクベースサイジング（損失額が capital × riskPct% に収まる株数）
	stopDist := math.Abs(price - plan.StopLoss)
	maxRiskUSD := capital * riskPct / 100.0
	sharesByRisk := 0.0
	if stopDist > 0 {
		sharesByRisk = maxRiskUSD / stopDist
	}
	// 2) ノミナル上限（HV補正後 = 総資本の最大15% × 係数）
	maxNominal := capital * 0.15 * hvFactor * confFactor
	sharesByNominal := 0.0
	if price > 0 {
		sharesByNominal = maxNominal / price
	}
	shares := math.Floor(math.Min(sharesByRisk, sharesByNominal))
	if shares < 0 {
		shares = 0
	}

	plan.Shares = int(shares)
	plan.PositionUSD = round2(shares * price)
	if capital > 0 {
		plan.PositionPct = round2(plan.PositionUSD / capital * 100)
	}
	plan.RiskUSD = round2(shares * stopDist)
	if capital > 0 {
		plan.RiskPctOfTotal = round2(plan.RiskUSD / capital * 100)
	}
	plan.RewardUSD = round2(shares * (plan.TakeProfit1 - price))
	if plan.RiskUSD > 0 {
		plan.RiskRewardRatio = round2(plan.RewardUSD / plan.RiskUSD)
	}
	plan.TimeStopDays = 15

	// ── Reasoning ─────────────────────────────────────────
	dirNote := ""
	if plan.Action == "BUY_SOXS" {
		dirNote = " (SOXS は逆連動なのでSOXL下落＝SOXS上昇)"
	}
	if sig.Score > 0 {
		plan.Reasoning = append(plan.Reasoning,
			"SOXLシグナルスコア "+fmtf(sig.Score)+" で**上昇優勢**。"+dirNote,
		)
	} else if sig.Score < 0 {
		plan.Reasoning = append(plan.Reasoning,
			"SOXLシグナルスコア "+fmtf(sig.Score)+" で**下落優勢**。"+dirNote,
		)
	} else {
		plan.Reasoning = append(plan.Reasoning,
			"SOXLシグナルスコア "+fmtf(sig.Score)+" で中立。"+dirNote,
		)
	}
	if rsi < 30 {
		plan.Reasoning = append(plan.Reasoning, "RSI "+fmtf(rsi)+" で売られすぎ → 反発期待。")
	} else if rsi > 70 {
		plan.Reasoning = append(plan.Reasoning, "RSI "+fmtf(rsi)+" で買われすぎ → 反落リスク。")
	} else {
		plan.Reasoning = append(plan.Reasoning, "RSI "+fmtf(rsi)+" で中立ゾーン。")
	}
	macdLast := ind.MACD.MACD[len(ind.MACD.MACD)-1]
	sigLast := ind.MACD.Signal[len(ind.MACD.Signal)-1]
	if macdLast > sigLast {
		plan.Reasoning = append(plan.Reasoning, "MACDがシグナルを上抜け（強気）。")
	} else {
		plan.Reasoning = append(plan.Reasoning, "MACDがシグナルを下抜け（弱気）。")
	}
	plan.Reasoning = append(plan.Reasoning,
		"損切り = エントリー − 1.5×ATR、利確 = +2.0×ATR と +3.0×ATR で機械的に決定。",
	)

	// ── Warnings ──────────────────────────────────────────
	if plan.Confidence == "弱" {
		plan.Warnings = append(plan.Warnings,
			"シグナルが弱め。エントリーを見送ってより明確な水準を待つのも有力。",
		)
	}
	if plan.RiskRewardRatio < 1.2 {
		plan.Warnings = append(plan.Warnings,
			"リスクリワード比が "+fmtf(plan.RiskRewardRatio)+" と低い。見送り推奨。",
		)
	}
	if shares == 0 {
		plan.Warnings = append(plan.Warnings,
			"資本が小さいか価格が高くて推奨株数が0株。サイズを再検討。",
		)
	}
	plan.Warnings = append(plan.Warnings,
		"3倍レバETFは1日±10%動くこともある。必ず損切りを置くこと。",
		"15営業日経っても利益が出なければ強制クローズ（時間ストップ）。",
	)

	return plan, nil
}

func lastNonZero(s []float64) float64 {
	for i := len(s) - 1; i >= 0; i-- {
		if !math.IsNaN(s[i]) && s[i] != 0 {
			return s[i]
		}
	}
	return 0
}

func round2(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*100) / 100
}

func fmtf(v float64) string {
	return strconv.FormatFloat(round2(v), 'f', 2, 64)
}
