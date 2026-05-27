// Frontend logic for the SOXL/SOXS dashboard.
const $ = (id) => document.getElementById(id);

const state = {
  period: "1y",
  ticker: "SOXL",
  capital: 10000,
};

function readControls() {
  state.period = $("period").value;
  state.ticker = $("ticker").value;
  state.capital = parseFloat($("capital").value) || 10000;
}

async function fetchJSON(url) {
  const r = await fetch(url);
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}

// ----------------- Tabs -----------------
document.querySelectorAll(".tab").forEach((t) => {
  t.addEventListener("click", () => {
    document.querySelectorAll(".tab").forEach((x) => x.classList.remove("active"));
    document.querySelectorAll(".panel").forEach((x) => x.classList.remove("active"));
    t.classList.add("active");
    $("tab-" + t.dataset.tab).classList.add("active");
  });
});

// ----------------- Helpers -----------------
const fmt = (v, d = 2) => (v == null || isNaN(v) ? "—" : Number(v).toFixed(d));
const pctClass = (v) => (v >= 0 ? "pos" : "neg");

function xy(bars, key) {
  return {
    x: bars.map((b) => b.time),
    y: bars.map((b) => b[key]),
  };
}

// ----------------- Plan Tab -----------------
let planForce = "SOXL"; // "SOXL" or "SOXS"
async function loadPlan() {
  const risk = $("risk_pct").value || 1.5;
  const forceParam = planForce ? `&force=${planForce}` : "";
  const d = await fetchJSON(`/api/plan?capital=${state.capital}&risk_pct=${risk}${forceParam}`);

  const card = $("plan_card");
  card.classList.remove("buy", "sell", "hold");
  if (d.action === "BUY_SOXL") card.classList.add("buy");
  else if (d.action === "BUY_SOXS") card.classList.add("sell");
  else card.classList.add("hold");

  $("plan_action").textContent = d.action_label;
  $("plan_label").textContent =
    d.action === "HOLD" ? "" : `推奨銘柄: ${d.ticker} · シグナル ${d.signal_score >= 0 ? "+" : ""}${d.signal_score}`;
  $("plan_conf").textContent = d.confidence !== "—" ? `確信度: ${d.confidence}` : "";

  if (d.action === "HOLD") {
    $("plan_grid").innerHTML = `<div class="item"><div class="k">推奨アクション</div><div class="v">現金保有</div></div>`;
  } else {
    $("plan_grid").innerHTML = `
      <div class="item entry"><div class="k">エントリー価格</div><div class="v">$${fmt(d.entry_price)}</div></div>
      <div class="item stop"><div class="k">損切り (Stop)</div><div class="v">$${fmt(d.stop_loss)}</div></div>
      <div class="item tp"><div class="k">利確1 (半分利確)</div><div class="v">$${fmt(d.take_profit_1)}</div></div>
      <div class="item tp"><div class="k">利確2 (残り)</div><div class="v">$${fmt(d.take_profit_2)}</div></div>
      <div class="item"><div class="k">推奨株数</div><div class="v">${d.shares} 株</div></div>
      <div class="item"><div class="k">投入額</div><div class="v">$${fmt(d.position_usd)}</div></div>
      <div class="item"><div class="k">資産比率</div><div class="v">${fmt(d.position_pct)}%</div></div>
      <div class="item"><div class="k">最大損失</div><div class="v neg">-$${fmt(d.risk_usd)}</div></div>
      <div class="item"><div class="k">期待利益 (TP1)</div><div class="v pos">+$${fmt(d.reward_usd)}</div></div>
      <div class="item"><div class="k">リスクリワード</div><div class="v">${fmt(d.risk_reward_ratio)}</div></div>
      <div class="item"><div class="k">ATR(14)</div><div class="v">$${fmt(d.atr)}</div></div>
      <div class="item"><div class="k">時間ストップ</div><div class="v">${d.time_stop_days}営業日</div></div>
    `;
  }

  $("plan_reasoning").innerHTML = (d.reasoning || [])
    .map((r) => `<li>${r}</li>`).join("");
  $("plan_warnings").innerHTML = (d.warnings || [])
    .map((w) => `<li>${w}</li>`).join("");
}

// ----------------- Chart Tab -----------------
async function loadCharts() {
  const data = await fetchJSON(
    `/api/prices?tickers=SOXL,SOXS,SOXX,SMH&period=${state.period}`
  );
  const traces = [];
  for (const [sym, s] of Object.entries(data)) {
    const base = s.bars[0].close;
    traces.push({
      x: s.bars.map((b) => b.time),
      y: s.bars.map((b) => (b.close / base) * 100),
      type: "scatter", mode: "lines", name: sym,
    });
  }
  Plotly.newPlot("chart_norm", traces, {
    paper_bgcolor: "#161b22", plot_bgcolor: "#161b22",
    font: { color: "#ccc" }, margin: { t: 20, l: 50, r: 20, b: 40 },
    hovermode: "x unified",
    yaxis: { title: "相対価格 (初日=100)" },
    legend: { orientation: "h", y: 1.1 },
  }, { responsive: true });

  const primary = data[state.ticker];
  if (!primary) return;
  Plotly.newPlot("chart_candle", [
    {
      type: "candlestick",
      x: primary.bars.map((b) => b.time),
      open: primary.bars.map((b) => b.open),
      high: primary.bars.map((b) => b.high),
      low: primary.bars.map((b) => b.low),
      close: primary.bars.map((b) => b.close),
      name: state.ticker, yaxis: "y",
    },
    {
      type: "bar",
      x: primary.bars.map((b) => b.time),
      y: primary.bars.map((b) => b.volume),
      marker: { color: "rgba(120,120,120,0.6)" },
      yaxis: "y2", name: "Volume",
    },
  ], {
    paper_bgcolor: "#161b22", plot_bgcolor: "#161b22",
    font: { color: "#ccc" }, margin: { t: 20, l: 50, r: 50, b: 40 },
    xaxis: { rangeslider: { visible: false } },
    yaxis: { domain: [0.3, 1.0], title: state.ticker },
    yaxis2: { domain: [0, 0.22], title: "Volume" },
    showlegend: false,
  }, { responsive: true });
}

// ----------------- Tech Tab -----------------
async function loadTech() {
  const d = await fetchJSON(`/api/indicators?ticker=${state.ticker}&period=${state.period}`);
  const bars = d.series.bars;
  const ind = d.indicators;
  const x = bars.map((b) => b.time);

  const traces = [
    { x, y: bars.map((b) => b.close), name: "Close", type: "scatter", mode: "lines" },
    { x, y: ind.sma20, name: "SMA20", line: { dash: "dot" } },
    { x, y: ind.sma50, name: "SMA50", line: { dash: "dot" } },
    { x, y: ind.bb.upper, name: "BB Upper", line: { color: "rgba(150,150,250,0.4)" } },
    { x, y: ind.bb.lower, name: "BB Lower", line: { color: "rgba(150,150,250,0.4)" }, fill: "tonexty" },
    { x, y: ind.rsi14, name: "RSI", yaxis: "y2", line: { color: "purple" } },
    { x, y: ind.macd.macd, name: "MACD", yaxis: "y3", line: { color: "#58a6ff" } },
    { x, y: ind.macd.signal, name: "Signal", yaxis: "y3", line: { color: "orange" } },
    { x, y: ind.macd.hist, name: "Hist", yaxis: "y3", type: "bar", marker: { color: "gray" } },
  ];

  const shapes = [
    // RSI 30/70 reference
    { type: "line", xref: "paper", x0: 0, x1: 1, yref: "y2", y0: 70, y1: 70, line: { color: "red", dash: "dash" } },
    { type: "line", xref: "paper", x0: 0, x1: 1, yref: "y2", y0: 30, y1: 30, line: { color: "green", dash: "dash" } },
  ];

  Plotly.newPlot("chart_tech", traces, {
    paper_bgcolor: "#161b22", plot_bgcolor: "#161b22",
    font: { color: "#ccc" }, margin: { t: 20, l: 60, r: 20, b: 40 },
    hovermode: "x unified",
    yaxis: { domain: [0.55, 1.0], title: "価格" },
    yaxis2: { domain: [0.30, 0.50], title: "RSI", range: [0, 100] },
    yaxis3: { domain: [0, 0.25], title: "MACD" },
    shapes,
    legend: { orientation: "h", y: 1.08 },
  }, { responsive: true });
}

// ----------------- Summary + Signal -----------------
async function loadSignal() {
  const d = await fetchJSON(`/api/signal?ticker=${state.ticker}&period=${state.period}`);
  $("m_close").textContent = "$" + fmt(d.close);
  $("m_chg").textContent = fmt(d.day_change) + "%";
  $("m_chg").className = "sub " + pctClass(d.day_change);
  $("m_score").textContent = (d.signal.score >= 0 ? "+" : "") + fmt(d.signal.score, 2);
  $("m_label").textContent = d.signal.label;
  $("m_rsi").textContent = fmt(d.rsi14, 1);
  $("m_hv").textContent = fmt(d.hv20, 1) + "%";
}

// ----------------- Decay Tab -----------------
async function loadDecay() {
  const d = await fetchJSON(`/api/decay?period=${state.period}&capital=${state.capital}`);
  const dec = d.soxl_decay;
  Plotly.newPlot("chart_decay", [
    { x: dec.map((p) => p.time), y: dec.map((p) => p.underlying_cum), name: "SOXX (原指数)", type: "scatter", mode: "lines" },
    { x: dec.map((p) => p.time), y: dec.map((p) => p.theoretical_cum), name: "理論3倍リターン", line: { dash: "dash" } },
    { x: dec.map((p) => p.time), y: dec.map((p) => p.actual_cum), name: "実SOXL", line: { color: "#f85149" } },
  ], {
    paper_bgcolor: "#161b22", plot_bgcolor: "#161b22",
    font: { color: "#ccc" }, margin: { t: 20, l: 60, r: 20, b: 40 },
    yaxis: { title: "累積リターン (初日=1)" },
    hovermode: "x unified",
  }, { responsive: true });

  const dual = d.dual;
  Plotly.newPlot("chart_dual", [
    { x: dual.map((p) => p.time), y: dual.map((p) => p.total), name: "合計資産", line: { color: "#f85149", width: 3 } },
    { x: dual.map((p) => p.time), y: dual.map((p) => p.soxl_value), name: "SOXL分", line: { dash: "dot" } },
    { x: dual.map((p) => p.time), y: dual.map((p) => p.soxs_value), name: "SOXS分", line: { dash: "dot" } },
  ], {
    paper_bgcolor: "#161b22", plot_bgcolor: "#161b22",
    font: { color: "#ccc" }, margin: { t: 20, l: 60, r: 20, b: 40 },
    yaxis: { title: "資産 (USD)" },
    shapes: [{ type: "line", xref: "paper", x0: 0, x1: 1, yref: "y", y0: state.capital, y1: state.capital, line: { color: "#666", dash: "dash" } }],
    hovermode: "x unified",
  }, { responsive: true });

  const finalPnl = dual[dual.length - 1].pnl_pct;
  $("dual_pnl").textContent = fmt(finalPnl, 2) + "%";
  $("dual_pnl").className = pctClass(finalPnl);
}

// ----------------- Backtest Tab -----------------
const BT_STRATEGY_DESC = {
  MACD: "<b>MACD戦略</b>: MACD(12,26,9)がシグナル線を上回っている間ロング保有。下回ったらクローズ。トレンドフォロー型。",
  RSI: "<b>RSI戦略</b>: RSI(14)が<b>30未満</b>で買い、<b>55超</b>で売り。逆張り型（売られすぎを拾う）。",
  MA_CROSS: "<b>移動平均クロス戦略</b>: SMA20 &gt; SMA50（ゴールデンクロス状態）の間ロング保有。下抜けでクローズ。古典的トレンドフォロー。",
};
function updateBtDesc() {
  const s = $("bt_strategy").value;
  $("bt_strategy_desc").innerHTML = BT_STRATEGY_DESC[s] || "";
}
async function runBacktest() {
  const strategy = $("bt_strategy").value;
  const fee = $("bt_fee").value;
  const d = await fetchJSON(
    `/api/backtest?ticker=${state.ticker}&period=${state.period}&strategy=${strategy}&capital=${state.capital}&fee_bps=${fee}`
  );
  const stats = d.result.stats;
  const labels = {
    total_return_pct: "総リターン (%)",
    cagr_pct: "CAGR (%)",
    sharpe: "Sharpe",
    max_dd_pct: "最大DD (%)",
    num_trades: "トレード数",
    win_rate_pct: "勝率 (%)",
    profit_factor: "PF",
  };
  $("bt_stats").innerHTML = Object.entries(labels)
    .map(([k, l]) => `<div><div class="k">${l}</div><div class="v">${stats[k]}</div></div>`)
    .join("");

  Plotly.newPlot("chart_bt", [
    { x: d.result.equity.map((p) => p.time), y: d.result.equity.map((p) => p.equity), name: "戦略", line: { color: "#3fb950", width: 2 } },
    { x: d.buy_hold.map((p) => p.time), y: d.buy_hold.map((p) => p.equity), name: "Buy & Hold", line: { color: "#888", dash: "dash" } },
  ], {
    paper_bgcolor: "#161b22", plot_bgcolor: "#161b22",
    font: { color: "#ccc" }, margin: { t: 20, l: 60, r: 20, b: 40 },
    yaxis: { title: "資産 (USD)" }, hovermode: "x unified",
  }, { responsive: true });

  const trades = d.result.trades || [];
  if (trades.length === 0) {
    $("bt_trades").innerHTML = "<p>トレードなし</p>";
  } else {
    $("bt_trades").innerHTML = `
      <table>
        <thead><tr><th>Entry</th><th>Exit</th><th>Entry Price</th><th>Exit Price</th><th>Return %</th></tr></thead>
        <tbody>
          ${trades
            .map(
              (t) =>
                `<tr><td>${t.entry.slice(0, 10)}</td><td>${t.exit.slice(0, 10)}</td><td>${fmt(t.entry_price)}</td><td>${fmt(t.exit_price)}</td><td class="${pctClass(t.return_pct)}">${fmt(t.return_pct)}%</td></tr>`
            )
            .join("")}
        </tbody>
      </table>`;
  }
}

// ----------------- News Tab -----------------
async function loadNews() {
  const d = await fetchJSON("/api/news");
  $("news_avg").textContent = fmt(d.avg_score, 2) + " " + d.label;
  $("news_count").textContent = d.count;
  $("news_list").innerHTML = (d.items || [])
    .map(
      (n) => `<div class="news-item">
        <div><strong>${n.sentiment}</strong> · <code>${n.ticker}</code> · <em>${n.publisher || ""}</em></div>
        <div><a href="${n.link}" target="_blank" rel="noopener">${escapeHtml(n.title)}</a></div>
        <div class="news-meta">${new Date(n.time).toLocaleString("ja-JP")}</div>
      </div>`
    )
    .join("");
}

function escapeHtml(s) {
  return s
    .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;").replace(/'/g, "&#39;");
}

// ----------------- Main -----------------
async function refreshAll() {
  readControls();
  try {
    await Promise.all([loadSignal(), loadPlan(), loadCharts(), loadTech(), loadDecay(), loadNews()]);
  } catch (e) {
    console.error(e);
    alert("読み込みエラー: " + e.message);
  }
}

$("refresh").addEventListener("click", refreshAll);
$("bt_run").addEventListener("click", runBacktest);
$("bt_strategy").addEventListener("change", updateBtDesc);
updateBtDesc();
function setPlanTicker(t) {
  planForce = t;
  $("plan_soxl").classList.toggle("active", t === "SOXL");
  $("plan_soxs").classList.toggle("active", t === "SOXS");
  readControls();
  loadPlan();
}
$("plan_soxl").addEventListener("click", () => setPlanTicker("SOXL"));
$("plan_soxs").addEventListener("click", () => setPlanTicker("SOXS"));

refreshAll();
