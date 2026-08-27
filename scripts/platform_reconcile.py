#!/usr/bin/env python3
"""
platform_reconcile.py — reconcile remote routatic-proxy usage against the
OpenCode platform usage page (authoritative billing).

Time semantics (verified 2026-08-27): platform `timeCreated` is the request
COMPLETION moment (matches proxy "streaming completed" log to the second);
remote `start_time` is the request START. Same request therefore differs by
its duration, so matching uses token triples (in, cr, out) with a wide
(+/-600s) window. Rows platform billed but the proxy never recorded (failed
but partially produced streams) are permanent one-sided gaps; this tool
backfills them (id = platform usage id, values as platform reports).

Usage:
  python3 scripts/platform_reconcile.py [--remote HOST] [--apply] [--day 2026-08-27]

Requires: running Edge with remote-debugging enabled (see edge-debug-attach
skill), SSH key to the remote host, and the opencode workspace tab open.
"""
import argparse
import asyncio
import csv
import json
import subprocess
import sys
from datetime import datetime, timezone, timedelta, date

WS = "ws://127.0.0.1:9222/devtools/browser/{uuid}"
FN_LIST = "bfd684bfc2e4eed05cd0b518f5e4eafd3f3376e3938abb9e536e7c03df831e5c"
WSID = "wrk_01KZAV780SFK2FEJXFRDZ0H34S"
REMOTE_DB = "/root/.local/share/routatic-proxy/data.db"


def parse_ts(s):
    s = s.strip('"').strip()
    if not s:
        return None
    try:
        return datetime.fromisoformat(s.replace('Z', '+00:00')).astimezone(timezone.utc)
    except ValueError:
        if '.' not in s:
            return None
        base, frac = s.split('.', 1)
        idx = max(frac.find('+'), frac.find('-'), frac.find('Z'))
        tz = frac[idx:] if idx >= 0 else ''
        ms = (frac[:idx] if idx >= 0 else frac)[:6]
        if not ms:
            return None
        try:
            return datetime.fromisoformat(f"{base}.{ms}{tz}".replace('Z', '+00:00')).astimezone(timezone.utc)
        except ValueError:
            return None


def bj_day(t):
    return (t + timedelta(hours=8)).date()


async def fetch_platform():
    port = open("/Users/zsj/Library/Application Support/Microsoft Edge/DevToolsActivePort").read().splitlines()
    import websockets
    ws_url = f"ws://127.0.0.1:{port[0]}/devtools/browser/{port[1].split('/')[-1]}"
    async with websockets.connect(ws_url, max_size=2 ** 29) as ws:
        pending, nid = {}, [0]
        async def reader():
            async for raw in ws:
                msg = json.loads(raw)
                if "id" in msg and msg["id"] in pending:
                    pending[msg["id"]].set_result(msg)
        rtask = asyncio.get_event_loop().create_task(reader())
        async def send(method, params=None, sid=None):
            nid[0] += 1
            msg = {"id": nid[0], "method": method, "params": params or {}}
            if sid:
                msg["sessionId"] = sid
            fut = asyncio.get_event_loop().create_future()
            pending[nid[0]] = fut
            await ws.send(json.dumps(msg))
            return await asyncio.wait_for(fut, 40)
        tg = await send("Target.getTargets", {})
        target = next((t["targetId"] for t in tg["result"]["targetInfos"]
                       if t["type"] == "page" and "opencode.ai/workspace" in t["url"]), None)
        if not target:
            raise SystemExit("NO opencode.ai workspace tab open")
        a = await send("Target.attachToTarget", {"targetId": target, "flatten": True})
        sid = a["result"]["sessionId"]
        await send("Page.bringToFront", {}, sid)
        flat, inst = [], [0]
        for page in range(200):
            inst[0] += 1
            expr = f"""(async () => {{
              const body = JSON.stringify({{ t: {{ t: 9, i: 0, l: 2, a: [{{t:1,s:{json.dumps(WSID)}}},{{t:0,s:{page}}}], o: 0 }}, f: 31, m: [] }});
              const r = await fetch('/_server', {{ method: 'POST', credentials: 'include',
                headers: {{ 'X-Server-Id': '{FN_LIST}', 'X-Server-Instance': 'server-fn:{inst[0]}', 'Content-Type': 'application/json' }}, body }});
              const text = await r.text(); eval(text);
              return JSON.stringify((self.$R || {{}})['server-fn:{inst[0]}'] ?? []);
            }})()"""
            r = await send("Runtime.evaluate", {"expression": expr, "returnByValue": True, "awaitPromise": True}, sid)
            val = r["result"]["result"].get("value")
            if not val or val in ("null", "[]"):
                break
            try:
                arr = json.loads(val)
            except Exception:
                break
            def walk(x):
                got = []
                if isinstance(x, dict):
                    if "id" in x and "timeCreated" in x:
                        got.append(x)
                    else:
                        for v in x.values():
                            got += walk(v)
                elif isinstance(x, list):
                    for i in x:
                        got += walk(i)
                return got
            recs = walk(arr)
            flat.extend(recs)
            if not recs:
                break
            await asyncio.sleep(0.2)
        rtask.cancel()
        seen = {}
        for rec in flat:
            seen.setdefault(rec["id"], rec)
        return list(seen.values())


def remote_rows(host):
    out = subprocess.run(["ssh", host, f"sqlite3 -csv {REMOTE_DB} "
                          "'SELECT id, start_time, cost_source, cost_usd, input_tokens, output_tokens, cache_read_tokens FROM requests;'"],
                         capture_output=True, text=True, check=True).stdout
    rows = []
    for r in csv.reader(out.splitlines()):
        if len(r) < 7:
            continue
        t = parse_ts(r[1])
        if t is None:
            continue
        try:
            rows.append({"id": r[0], "t": t, "src": r[2], "c": float(r[3]),
                         "i": int(r[4] or 0), "o": int(r[5] or 0), "cr": int(r[6] or 0)})
        except ValueError:
            continue
    return rows


def reconcile(plat_rows, rem_rows, day):
    panel = [r for r in plat_rows if bj_day(parse_ts(r.get("timeCreated", ""))) == day]
    rem = [r for r in rem_rows if bj_day(r["t"]) == day]
    plat = [{"id": r["id"], "t": parse_ts(r["timeCreated"]), "c": r.get("cost", 0) / 1e8,
             "i": r.get("inputTokens", 0), "o": r.get("outputTokens", 0),
             "cr": r.get("cacheReadTokens") or 0} for r in panel]
    used_p, used_r = set(), set()
    for rr in rem:
        for pp in plat:
            if pp["id"] in used_p:
                continue
            if rr["i"] == pp["i"] and rr["o"] == pp["o"] and rr["cr"] == pp["cr"] \
                    and abs((rr["t"] - pp["t"]).total_seconds()) <= 600:
                used_p.add(pp["id"])
                used_r.add(rr["id"])
                break
    plat_only = [p for p in plat if p["id"] not in used_p]
    rem_only = [r for r in rem if r["id"] not in used_r]
    print(f"day {day}: remote {len(rem)} rows ${sum(r['c'] for r in rem):.4f} | "
          f"platform {len(plat)} rows ${sum(p['c'] for p in plat):.4f}")
    print(f"  matched {len(used_r)} | platform-only {len(plat_only)} "
          f"${sum(p['c'] for p in plat_only):.4f} | remote-only {len(rem_only)} "
          f"${sum(r['c'] for r in rem_only):.4f}")
    # Amount-level cancel: the same bill expressed with different token splits
    # (proxy keeps full input count, platform reports cache-stripped input) never
    # token-matches, but the amounts are the authoritative equal side.
    po = [p for p in plat_only]
    ro = [r for r in rem_only]
    cancelled = 0
    for p in list(po):
        for r in list(ro):
            if abs(p["c"] - r["c"]) < 1e-9:
                print(f"    AMOUNT-MATCH {p['id']} (${p['c']:.6f}) <-> {r['id']} "
                      f"({r['t'].strftime('%H:%M:%S')}Z) — same bill, split differs")
                po.remove(p)
                ro.remove(r)
                cancelled += 1
                break
    for p in po:
        print(f"    PLAT-ONLY {p['id']} {p['t'].strftime('%H:%M:%S')}Z in={p['i']} cr={p['cr']} out={p['o']} ${p['c']:.6f}")
    for r in ro:
        print(f"    REM-ONLY  {r['id']} {r['t'].strftime('%H:%M:%S')}Z {r['src']} ${r['c']:.6f}")
    if cancelled:
        print(f"  {cancelled} pair(s) cancelled by equal amount — totals align")
    return po, ro


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--remote", default="root@23.80.89.173")
    ap.add_argument("--apply", action="store_true", help="insert platform-only rows into remote DB")
    ap.add_argument("--day", default="2026-08-27")
    args = ap.parse_args()
    day = date.fromisoformat(args.day)
    plat_rows = asyncio.run(fetch_platform())
    rem_rows = remote_rows(args.remote)
    plat_only, _ = reconcile(plat_rows, rem_rows, day)
    if not args.apply or not plat_only:
        sys.exit(0 if not plat_only else 1)
    for p in plat_only:
        st = (p["t"] + timedelta(hours=8)).strftime("%Y-%m-%dT%H:%M:%S+08:00")
        sql = (f"INSERT OR IGNORE INTO requests (id, model, provider, scenario, start_time, "
               f"duration_ms, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, "
               f"cost_usd, cost_source, details_known, streaming, success, error_msg, attempt, peak_multiplier) "
               f"VALUES ('{p['id']}','deepseek-v4-flash','opencode-go','override','{st}',0,"
               f"{p['i']},{p['o']},{p['cr']},0,{p['c']:.8f},'provider',1,0,0,"
               f"'failed stream billed by platform',1,1);")
        subprocess.run(["ssh", args.remote, f"sqlite3 {REMOTE_DB} \"{sql}\""], check=True)
        print(f"  backed up? no — inserted {p['id']} ${p['c']:.6f}")


if __name__ == "__main__":
    main()
