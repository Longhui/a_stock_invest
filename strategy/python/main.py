"""
tail_20_2 隔夜交易策略 — BulletTrade LiveEngine 入口

逻辑:
  1. 每天调用 Go 引擎选股 (XG1+XG2 条件)
  2. 尾盘买入候选，最多 4 只各 20%
  3. 不在候选或触发止损 → 卖出

用法 (模拟盘):
  cd strategy/python
  bullet-trade live main.py --broker simulator

用法 (实盘):
  # 复制 .env 为 .env.live, 修改 DEFAULT_BROKER=qmt
  bullet-trade live main.py --broker qmt
"""

import os
import sys

sys.path.insert(0, os.path.dirname(__file__))
from strategy_bridge import run_go_engine, get_candidates

from jqdata import *


def initialize(context):
    """策略初始化。"""
    set_benchmark("000300.XSHG")
    set_option("use_real_price", True)
    set_option("avoid_future_data", True)

    set_slippage(FixedSlippage(0.002))
    set_order_cost(OrderCost(
        open_tax=0,
        close_tax=0.001,
        open_commission=0.0003,
        close_commission=0.0003,
        min_commission=5,
    ), type="stock")

    # ----- 策略参数 -----
    g.max_positions = 4          # 最多同时持仓数
    g.position_pct = 0.20        # 单只仓位占比
    g.stop_loss_pct = -0.02      # 2% 止损线

    # ----- 调度 -----
    # 开盘: 卖出不在候选或触止损的持仓
    run_daily(sell_positions, time="open")
    # 尾盘: 买入候选
    run_daily(buy_candidates, time="14:50")


def sell_positions(context):
    """开盘卖出：不在今日候选或触发止损的持仓。"""
    today = context.current_dt.strftime("%Y-%m-%d")
    data = run_go_engine(today)
    if data is None:
        log.warning("Go 引擎失败，跳过卖出")
        return

    # MACD 为绿 → 无条件清仓
    if not data.get("macd_ok"):
        log.warning("大盘 MACD 为绿，清仓")
        for stock in list(context.portfolio.positions.keys()):
            order_target(stock, 0)
        return

    target_codes = {c["code"] for c in data.get("candidates", [])}

    for stock in list(context.portfolio.positions.keys()):
        reasons = []

        # 不在候选列表
        if stock not in target_codes:
            reasons.append("不在候选")

        # 止损
        pos = context.portfolio.positions[stock]
        if hasattr(pos, "avg_cost") and pos.avg_cost > 0:
            current_price = pos.price if hasattr(pos, "price") else pos.avg_cost
            pnl_pct = (current_price / pos.avg_cost) - 1
            if pnl_pct <= g.stop_loss_pct:
                reasons.append(f"止损 ({pnl_pct:.1%})")

        if reasons:
            log.info(f"卖出 {stock}: {'; '.join(reasons)}")
            order_target(stock, 0)


def buy_candidates(context):
    """尾盘买入：按风控评分顺序开仓。"""
    today = context.current_dt.strftime("%Y-%m-%d")
    data = run_go_engine(today)
    if data is None:
        log.warning("Go 引擎失败，跳过买入")
        return

    if not data.get("macd_ok"):
        log.info("大盘 MACD 为绿，跳过买入")
        return

    candidates = get_candidates(data)
    if not candidates:
        log.info("今日无候选")
        return

    # 当前持仓中属于候选的部分
    target_codes = {c["code"] for c in candidates}
    held_codes = set(context.portfolio.positions.keys()) & target_codes
    available = g.max_positions - len(held_codes)
    if available <= 0:
        log.info("仓位已满")
        return

    # 逐只买入
    bought = 0
    for c in candidates:
        if bought >= available:
            break
        code = c["code"]
        if code in held_codes:
            continue  # 已持仓，跳过

        target_value = context.portfolio.total_value * g.position_pct
        log.info(
            "买入 %s %s  评分=%.0f  建议=%s  目标=%.0f",
            code, c.get("name", ""),
            c.get("score", 0),
            c.get("suggestion", ""),
            target_value,
        )
        order_target_value(code, target_value)
        bought += 1

    if bought > 0:
        log.info("本日买入 %d 只", bought)
