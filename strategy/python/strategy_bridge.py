"""
tail_20_2 Go 引擎桥接模块

调用 Go 选股引擎 (-output json)，解析候选列表。
可脱离 BulletTrade 独立测试。
"""

import json
import subprocess
import os
import sys
from typing import Optional

# main.go 所在目录（strategy/）
GO_MAIN_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))


def run_go_engine(
    date_str: str,
    skip_macd: bool = False,
    timeout: int = 120,
) -> Optional[dict]:
    """调用 Go 引擎获取今日选股结果。

    Args:
        date_str: 目标日期 YYYY-MM-DD
        skip_macd: 跳过 MACD 检查
        timeout: 超时秒数

    Returns:
        解析后的 BridgeOutput dict；失败返回 None
    """
    cmd = [
        "go", "run", "main.go",
        "-date", date_str,
        "-output", "json",
    ]
    if skip_macd:
        cmd.append("-skip-macd")

    try:
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            cwd=GO_MAIN_DIR,
            timeout=timeout,
        )
    except FileNotFoundError:
        print("[bridge] Go 未安装或不在 PATH 中")
        return None
    except subprocess.TimeoutExpired:
        print(f"[bridge] Go 引擎超时 ({timeout}s) — 扫描股票过多?")
        return None

    if result.returncode != 0:
        print(f"[bridge] Go 引擎调用失败 (exit={result.returncode})")
        if result.stderr:
            for line in result.stderr.strip().split("\n"):
                print(f"  stderr: {line}")
        return None

    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError as e:
        print(f"[bridge] Go 输出解析失败: {e}")
        print(f"  stdout: {result.stdout[:500]}")
        return None


def get_candidates(data: dict) -> list:
    """从 Go 引擎输出中提取按风控评分降序排列的候选列表。

    Args:
        data: run_go_engine() 返回的 dict

    Returns:
        候选列表（已排序），空列表表示无候选
    """
    if not data or not data.get("macd_ok"):
        return []
    candidates = list(data.get("candidates", []))
    candidates.sort(key=lambda c: c.get("score", 0), reverse=True)
    return candidates


def main_cli():
    """命令行独立测试入口。

    Usage:
        python strategy_bridge.py 2026-07-10
        python strategy_bridge.py 2026-07-10 --skip-macd
    """
    date_str = sys.argv[1] if len(sys.argv) > 1 else "2026-07-10"
    skip_macd = "--skip-macd" in sys.argv

    data = run_go_engine(date_str, skip_macd=skip_macd)
    if data is None:
        sys.exit(1)

    print(json.dumps(data, indent=2, ensure_ascii=False))
    sys.exit(0)


if __name__ == "__main__":
    main_cli()
