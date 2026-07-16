# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A-stock (A股) quantitative trading strategy system in Go. Implements the `tail_20_2` overnight trading strategy: T-day close buy → T+1 sell, with 20% position × 4 stocks × 2% stop-loss. Data sourced from 通达信 (Tongdaxin) desktop software.

## Commands

```bash
# Run real-time stock selection (default date 2026-07-10)
cd strategy && go run main.go

# With options
go run main.go -date 2026-07-14 -skip-macd -debug

# Run backtest
go run cmd/backtest/main.go

# Run all tests
go test ./...

# Run single package tests
go test ./internal/riskcontrol/...
go test ./internal/selector/...
go test ./internal/sellcondition/...

# Build
go build ./...
```

## Project Structure

```
strategy/
├── main.go                          # Real-time stock selection entry point
├── cmd/
│   ├── backtest/main.go             # Backtesting entry point
│   └── tail_20_2/main.go            # Strategy-specific backtest entry
├── internal/
│   ├── provider/provider.go         # Data provider: local TDX .day files + tdx-api network fallback + caching
│   ├── reader/
│   │   ├── dayfile.go              # Binary .day file read/write (32-byte records), MA/EMA/SMA/StdDev helpers
│   │   ├── block.go                # TDX BlockMapXML.dat parsing, sector mapping
│   │   └── circulate.go            # Circulate shares from Tencent stock API (qt.gtimg.cn)
│   ├── selector/
│   │   ├── context.go              # Technical computation context (MA, MACD, KDJ, RSV, Winner, Cross)
│   │   ├── screen.go               # ScreenStock() entry point
│   │   └── strategy.go             # Core stock selection logic (XG1 + XG2 dual conditions)
│   ├── strategy/tail_20_2/
│   │   ├── config.go               # Strategy config (default: 1M capital, 20%×4, 2% stop-loss)
│   │   ├── types.go                # Data types (Kline, StockDataCache, CandidateResult, SellDecision)
│   │   ├── market.go               # Market data loading (all stocks, index 60min klines, board filter)
│   │   └── strategy.go             # Strategy logic (MACD check, sell decision, candidate selection, risk input building)
│   ├── riskcontrol/
│   │   ├── types.go                # Risk types (RiskInput, RiskResult, RiskSummary, RiskColor🟢🟡🔴)
│   │   ├── engine.go               # Risk pipeline orchestrator + dual-weight sorting
│   │   ├── rules.go                # 5-step scoring rules + finalization
│   │   ├── sector.go               # Sector/topic classification heuristics
│   │   └── output.go               # Report formatting
│   ├── backtest/
│   │   ├── types.go                # Backtest types (Config, Position, Trade, DailyValue, Summary)
│   │   ├── engine.go               # Day-by-day simulation engine
│   │   └── reporter.go             # Report output (detail, monthly, compact)
│   └── sellcondition/
│       └── sellcondition.go        # Intraday take-profit/stop-loss monitoring via 1-min klines
```

## Architecture & Data Flow

### Data Source Hierarchy
1. **Local TDX .day files** (`vipdoc/{sh,sz,bj}/lday/`) — primary, fastest
2. **Local cache** (`T0002/cache/cache_klines/`) — saved API results in .day format
3. **tdx-api network fallback** (`github.com/injoyai/tdx`, replaced by local `../tdx-api`) — TCP protocol to TDX servers
4. **Tencent stock API** (`qt.gtimg.cn`) — for circulate shares data
5. **East Money API** (`push2.eastmoney.com`) — for sector fund flow data

### Real-time Selection Flow
```
main.go → GetAllStockCodes() → FilterByBoard() → CheckMACDLive() →
SelectCandidatesLive() [8 workers, per stock: GetStockData → ScreenStock] →
BuildRiskInputs() → riskcontrol.ProcessAll() → rank & output
```

### Backtest Flow
```
backtest.Engine.Run():
  for each trading day:
    1. sellPositions() → tail_20_2.CheckSell() for stop-loss/take-profit/close
    2. CheckMACD() for market environment
    3. buyStocks() → SelectCandidates() → BuildRiskInputs() → ProcessAll() → execute buy
    4. recordDailyValue()
```

### Stock Selection Logic (`selector.strategy.go`)
Two condition sets must both pass:
- **XG1**: Yang line + above MA5 + volume expansion + MACD bullish + KDJ not overbought + price limit + chip filter
- **XG2**: Golden cross (MA5/MA10 or KDJ or MACD DIF/DEA) + turnover 3.5-15%
- Plus: no limit-up yesterday + K-line structure (preceding yin + 3/4 consecutive yang)

### Risk Control Pipeline (`riskcontrol`)

5-step scoring: sector sentiment → technical rating (🟢🟡🔴) → volume scoring → open prediction → finalization. Dual-weight sort: OvernightRisk (ascending) → PulseScore (descending). Market cap >100B always ranks after small-cap.

## Key Dependencies

- `github.com/injoyai/tdx` — replaced via `go.mod replace` directive to local `../tdx-api`
- TDX (通达信) desktop installation required at `D:/Programs/tdx`
- Tencent stock API (`qt.gtimg.cn`) for fundamental data
- East Money API for sector fund flow

## Testing Patterns

Tests use real market data (no mocks). `screen_test.go` tests with hardcoded kline data in `stock-data/`. `integration_test.go` tests actual TDX connection. `sellcondition_test.go` tests intraday sell logic with simulated minute klines.
