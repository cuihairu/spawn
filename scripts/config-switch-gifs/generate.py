#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""DOC-GIF 配置开关效果动态图流水线（kode C-24 同款模式，spawn 版）。

对 switches.json 里 capture=true 的每个开关：起真实服务（SQLite 临时库），
先关后开各跑一遍演示请求，收集真实响应文本做行为断言（兼作配置效果回归
测试——断言不过直接红，宁可不产图），用 PIL 渲染终端帧，ffmpeg 缝循环
GIF，产物落 docs/public/config-switch-gifs/，清单写 manifest.json。

用法：
  python3 scripts/config-switch-gifs/generate.py --list
  python3 scripts/config-switch-gifs/generate.py --switch=community-blocked-words
  python3 scripts/config-switch-gifs/generate.py --all
  python3 scripts/config-switch-gifs/generate.py --check   # 产物/清单/文档一致性闸

依赖：Go 工具链（编译各服务）、ffmpeg、Python PIL、Noto CJK 字体。
"""

import argparse
import base64
import hashlib
import hmac
import http.server
import json
import os
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.request
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "docs" / "public" / "config-switch-gifs"
MANIFEST = OUT / "manifest.json"
WORK = Path(os.environ.get("SPAWN_GIF_WORK", "/tmp/spawn-gif-capture"))

JWT_SECRET = "gif-demo-jwt-secret"
INTERNAL_TOKEN = "gif-internal-token"
WEBHOOK_PORT = 18997

FONT_CANDIDATES = [
    "/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
    "/usr/share/fonts/opentype/noto/NotoSansCJK-Bold.ttc",
]

FRAME_W, FRAME_H = 980, 560
MARGIN_X, MARGIN_Y = 26, 52
LINE_H = 30
FONT_SIZE = 19
BG = (16, 20, 24)
BAR_BG = (30, 36, 44)
FG = (226, 230, 235)
MUTED = (122, 132, 144)
PROMPT = (110, 200, 130)
OK_GREEN = (110, 200, 130)
ERR_RED = (240, 110, 110)
ACCENT = (232, 113, 10)

# 每服务 go-zero/裸 HTTP 服务的配置模板（SQLite 临时库，端口固定、互不冲突）。
CONFIG_TEMPLATES = {
    "community": """Name: community-api
Host: 127.0.0.1
Port: 18992
Timeout: 30000
MySQL:
  DataSource: "file:{db}"
Auth:
  JWTSecret: {jwt}
Moderation:
  BlockedWords: {blocked_words}
  AdminUserIds: {admin_ids}
Realtime:
  Url: ""
  InternalToken: ""
Internal:
  Token: "{internal_token}"
Log:
  ServiceName: community-api
  Mode: console
  Encoding: json
  Level: error
""",
    "data-panel": """Name: data-panel-api
Host: 127.0.0.1
Port: 18996
Timeout: 30000
MySQL:
  DataSource: "file:{db}"
Auth:
  JWTSecret: {jwt}
Internal:
  Token: "{internal_token}"
Analytics:
  AdminUserIds: {analytics_admin_ids}
GameCatalog:
  BaseURL: "http://127.0.0.1:18990"
  Timeout: 5000
Log:
  ServiceName: data-panel-api
  Mode: console
  Encoding: json
  Level: error
""",
    "game-catalog": """Name: game-catalog-api
Host: 127.0.0.1
Port: 18990
Timeout: 30000
MySQL:
  DataSource: "file:{db}"
Auth:
  JWTSecret: {jwt}
Package:
  Dir: "{package_dir}"
Internal:
  Token: "{internal_token}"
Notify:
  Url: ""
  Token: ""
Analytics:
  BaseURL: ""
  Token: ""
Log:
  ServiceName: game-catalog-api
  Mode: console
  Encoding: json
  Level: error
""",
    "realtime-hub": """Listen: "127.0.0.1:18993"
WriteTimeoutMs: 10000
PingIntervalMs: 30000
SendQueueSize: 16
JwtSecret: {jwt}
InternalToken: "{internal_token}"
RedisAddr: ""
RedisPassword: ""
PushWebhook: "{push_webhook}"
PushToken: ""
""",
}

# 每开关 × off/on 的模板填充值（与 switches.json 的 config 路径一一对应）。
STATE_VALUES = {
    "community-blocked-words": {
        "off": {"blocked_words": "[]"},
        "on": {"blocked_words": '["违禁演示词"]'},
    },
    "community-moderation-admin-ids": {
        "off": {"admin_ids": "[1]"},
        "on": {"admin_ids": "[1, 7]"},
    },
    "community-internal-token": {
        "off": {"internal_token": ""},
        "on": {"internal_token": INTERNAL_TOKEN},
    },
    "data-panel-internal-token": {
        "off": {"internal_token": ""},
        "on": {"internal_token": INTERNAL_TOKEN},
    },
    "data-panel-analytics-admin-ids": {
        "off": {"analytics_admin_ids": "[]"},
        "on": {"analytics_admin_ids": '["7"]'},
    },
    "game-catalog-internal-token": {
        "off": {"internal_token": ""},
        "on": {"internal_token": INTERNAL_TOKEN},
    },
    "realtime-hub-internal-token": {
        "off": {"internal_token": ""},
        "on": {"internal_token": INTERNAL_TOKEN},
    },
    "realtime-hub-push-webhook": {
        "off": {"internal_token": INTERNAL_TOKEN, "push_webhook": ""},
        "on": {"internal_token": INTERNAL_TOKEN, "push_webhook": f"http://127.0.0.1:{WEBHOOK_PORT}/push"},
    },
}


# ---------- 基础工具 ----------

def die(msg):
    print(f"[config-switch-gifs] FATAL: {msg}", file=sys.stderr)
    sys.exit(1)


def load_switches():
    data = json.loads((Path(__file__).parent / "switches.json").read_text(encoding="utf-8"))
    return data


def jwt_for(user_id, username="gif-user"):
    """手卷 HS256 JWT（community/utils.JWTClaims 同款 claims）。"""
    def b64(raw):
        return base64.urlsafe_b64encode(raw).rstrip(b"=")

    now = int(time.time())
    header = b64(json.dumps({"alg": "HS256", "typ": "JWT"}).encode())
    payload = b64(json.dumps({
        "user_id": user_id,
        "username": username,
        "exp": now + 3600,
        "iat": now,
        "nbf": now,
    }, separators=(",", ":")).encode())
    signing = header + b"." + payload
    sig = b64(hmac.new(JWT_SECRET.encode(), signing, hashlib.sha256).digest())
    return (signing + b"." + sig).decode()


def http_call(method, url, body=None, headers=None):
    """真实请求；返回 (status, body_text 全文)。展示侧自行 clip()。"""
    req = urllib.request.Request(url, method=method, data=(body.encode() if body else None))
    req.add_header("Content-Type", "application/json")
    for k, v in (headers or {}).items():
        req.add_header(k, v)
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            status = resp.status
            text = resp.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        status = e.code
        text = e.read().decode("utf-8", "replace")
    except urllib.error.URLError as e:
        return 0, f"request failed: {e.reason}"
    return status, text


def clip(text, n=72):
    return text if len(text) <= n else text[:n] + "…"


def wait_port(port, timeout=20):
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            with socket.create_connection(("127.0.0.1", port), timeout=0.5):
                return True
        except OSError:
            time.sleep(0.15)
    return False


def free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname().port


# ---------- 服务进程 ----------

def build_binary(svc):
    meta = svc["meta"]
    bin_path = WORK / "bin" / meta["binary"]
    bin_path.parent.mkdir(parents=True, exist_ok=True)
    cmd = ["go", "build", "-o", str(bin_path), "."]
    print(f"[build] {meta['dir']} -> {bin_path}")
    subprocess.run(cmd, cwd=ROOT / meta["dir"], check=True)
    return bin_path


def render_config(service, db_path, values, package_dir):
    tpl = CONFIG_TEMPLATES[service]
    fill = {
        "db": db_path,
        "jwt": JWT_SECRET,
        "blocked_words": "[]",
        "admin_ids": "[1]",
        "analytics_admin_ids": "[]",
        "internal_token": "",
        "package_dir": package_dir,
        "push_webhook": "",
    }
    fill.update(values)
    return tpl.format(**fill)


def run_state(svc, switch_key, state, values, demo):
    """起服务跑演示并回收：返回 (transcript_lines)。失败直接 die。"""
    meta = svc["meta"]
    run_dir = WORK / "runs" / f"{switch_key}-{state}"
    shutil.rmtree(run_dir, ignore_errors=True)
    run_dir.mkdir(parents=True, exist_ok=True)
    db_path = run_dir / "db.sqlite"
    package_dir = run_dir / "packages"
    package_dir.mkdir(exist_ok=True)
    cfg_path = run_dir / "config.yaml"
    cfg_path.write_text(render_config(meta["service"], db_path, values, package_dir), encoding="utf-8")

    bin_path = WORK / "bin" / meta["binary"]
    port = meta["port"]
    log_path = run_dir / "service.log"
    with open(log_path, "wb") as logf:
        proc = subprocess.Popen(
            [str(bin_path), "-f", str(cfg_path)] if meta["service"] != "realtime-hub"
            else [str(bin_path), "-f", str(cfg_path)],
            stdout=logf, stderr=subprocess.STDOUT,
            start_new_session=True,
        )
    try:
        if not wait_port(port):
            log = log_path.read_text("utf-8", "replace")[-2000:]
            die(f"{switch_key}/{state}: service did not listen on {port}\n--- log tail ---\n{log}")
        lines = demo(state, values)
    finally:
        os.killpg(proc.pid, signal.SIGTERM)
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            os.killpg(proc.pid, signal.SIGKILL)
    return lines


# ---------- 演示脚本（真实请求 + 行为断言 = 回归门） ----------

def assert_eq(actual, want, what):
    if actual != want:
        die(f"behavior regression: {what} = {actual!r}, want {want!r}")


def demo_community_blocked_words(state, values, base):
    content = '{"topic_id":1,"title":"演示帖","content":"这条内容包含违禁演示词请留意"}'
    auth = {"Authorization": "Bearer " + jwt_for(42)}
    code, body = http_call("POST", f"{base}/api/v1/posts", content, auth)
    if state == "off":
        assert_eq(code, 200, "blocked-words off create post")
        return [
            ("comment", "# 演示词「违禁演示词」不在词表，发帖成功"),
            ("cmd", 'curl -X POST :18992/api/v1/posts \\'),
            ("out", '    -d \'{"topic_id":1,"title":"演示帖",'),
            ("out", '     "content":"这条内容包含违禁演示词请留意"}\''),
            ("ok", f"HTTP {code}  {clip(body, 64)}"),
            ("comment", "# BlockedWords: [] → 表空回落内置默认词表，演示词不在其中"),
        ]
    assert_eq(code, 400, "blocked-words on create post")
    return [
        ("comment", "# BlockedWords 配置演示词后重启，同样请求被拒"),
        ("cmd", 'curl -X POST :18992/api/v1/posts \\'),
        ("out", '    -d \'{"topic_id":1,"title":"演示帖",'),
        ("out", '     "content":"这条内容包含违禁演示词请留意"}\''),
        ("err", f"HTTP {code}  {clip(body, 64)}"),
    ]


def demo_community_admin_ids(state, values, base):
    auth42 = {"Authorization": "Bearer " + jwt_for(42)}
    code, body = http_call("POST", f"{base}/api/v1/topics", '{"name":"gif-demo-topic","description":"gif demo"}', auth42)
    assert_eq(code, 200, "admin-ids create topic")
    topic_id = json.loads(body or "{}").get("topic", {}).get("id", 1)
    post_body = json.dumps({"topic_id": topic_id, "title": "待置顶帖", "content": "正文"}, ensure_ascii=False)
    code, body = http_call("POST", f"{base}/api/v1/posts", post_body, auth42)
    assert_eq(code, 200, "admin-ids create post")
    post_id = json.loads(body or "{}").get("post", {}).get("id", 1)
    auth7 = {"Authorization": "Bearer " + jwt_for(7)}
    code, body = http_call("POST", f"{base}/api/v1/posts/{post_id}/pin",
                           '{"pinned":true}', auth7)
    if state == "off":
        assert_eq(code, 403, "admin-ids off pin")
        return [
            ("comment", "# 用户 42 建圈发帖成功；用户 7（名单外）尝试置顶"),
            ("cmd", "curl -X POST :18992/api/v1/posts/%d/pin \\" % post_id),
            ("out", "    -H 'Authorization: Bearer <user-7-jwt>' -d '{\"pinned\":true}'"),
            ("err", f"HTTP {code}  {clip(body, 64) or '(forbidden)'}"),
            ("comment", "# AdminUserIds: [1] → 用户 7 不在名单，403"),
        ]
    assert_eq(code, 200, "admin-ids on pin")
    return [
        ("comment", "# AdminUserIds: [1, 7] 后重启，同样置顶请求"),
        ("cmd", "curl -X POST :18992/api/v1/posts/%d/pin \\" % post_id),
        ("out", "    -H 'Authorization: Bearer <user-7-jwt>' -d '{\"pinned\":true}'"),
        ("ok", f"HTTP {code}  {clip(body, 64)}"),
    ]


def demo_community_internal_token(state, values, base):
    body_req = '{"user_ids":[42],"type":"system","content":"版本公告"}'
    headers = {"X-Internal-Token": INTERNAL_TOKEN}
    code, body = http_call("POST", f"{base}/internal/notifications", body_req, headers)
    if state == "off":
        assert_eq(code, 403, "community internal off")
        return [
            ("comment", "# Internal.Token 未配置：任何令牌都进不来（fail-closed）"),
            ("cmd", "curl -X POST :18992/internal/notifications \\"),
            ("out", "    -H 'X-Internal-Token: gif-internal-token' \\"),
            ("out", "    -d '{\"user_ids\":[42],\"type\":\"system\",...'"),
            ("err", f"HTTP {code}"),
            ("comment", "# 令牌校验先于 body 解析，未配置恒 403"),
        ]
    assert_eq(code, 200, "community internal on")
    return [
        ("comment", "# Internal.Token: gif-internal-token 后重启"),
        ("cmd", "curl -X POST :18992/internal/notifications \\"),
        ("out", "    -H 'X-Internal-Token: gif-internal-token' \\"),
        ("out", "    -d '{\"user_ids\":[42],\"type\":\"system\",...'"),
        ("ok", f"HTTP {code}  {clip(body, 64)}"),
    ]


def demo_data_panel_internal_token(state, values, base):
    body_req = '{"events":[{"event_id":"gif-001","event_type":"order_paid","amount_cents":990}]}'
    headers = {"X-Internal-Token": INTERNAL_TOKEN}
    code, body = http_call("POST", f"{base}/internal/events", body_req, headers)
    if state == "off":
        assert_eq(code, 403, "data-panel internal off")
        return [
            ("comment", "# 摄入口令未配置：/internal/events 整体拒绝"),
            ("cmd", "curl -X POST :18996/internal/events \\"),
            ("out", "    -H 'X-Internal-Token: gif-internal-token' \\"),
            ("out", "    -d '{\"events\":[{\"event_id\":\"gif-001\","),
            ("out", "            \"event_type\":\"order_paid\",\"amount_cents\":990}]}'"),
            ("err", f"HTTP {code}"),
        ]
    assert_eq(code, 200, "data-panel internal on")
    created = json.loads(body or "{}").get("accepted", "?")
    return [
        ("comment", "# Internal.Token 配置后重启，事实事件入库"),
        ("cmd", "curl -X POST :18996/internal/events \\"),
        ("out", "    -H 'X-Internal-Token: gif-internal-token' \\"),
        ("out", "    -d '{\"events\":[{\"event_id\":\"gif-001\","),
        ("out", "            \"event_type\":\"order_paid\",\"amount_cents\":990}]}'"),
        ("ok", f"HTTP {code}  {{\"accepted\":{created}}}"),
    ]


def demo_data_panel_analytics_admin(state, values, base):
    auth = {"Authorization": "Bearer " + jwt_for(7)}
    code, body = http_call("GET", f"{base}/api/v1/analytics/overview?from=2026-01-01&to=2026-12-31",
                           None, auth)
    if state == "off":
        assert_eq(code, 403, "analytics admin off")
        return [
            ("comment", "# 用户 7 查询经营分析：名单为空全关"),
            ("cmd", "curl ':18996/api/v1/analytics/overview?from=2026-01-01&to=2026-12-31' \\"),
            ("out", "    -H 'Authorization: Bearer <user-7-jwt>'"),
            ("err", f"HTTP {code}  {clip(body, 64) or '(forbidden)'}"),
        ]
    assert_eq(code, 200, "analytics admin on")
    return [
        ("comment", "# Analytics.AdminUserIds: [\"7\"] 后重启"),
        ("cmd", "curl ':18996/api/v1/analytics/overview?from=2026-01-01&to=2026-12-31' \\"),
        ("out", "    -H 'Authorization: Bearer <user-7-jwt>'"),
        ("ok", f"HTTP {code}  {clip(body, 64)}"),
    ]


def demo_game_catalog_internal_token(state, values, base):
    headers = {"X-Internal-Token": INTERNAL_TOKEN}
    code, body = http_call("GET", f"{base}/internal/games/1/reservation-users", None, headers)
    if state == "off":
        assert_eq(code, 403, "game-catalog internal off")
        return [
            ("comment", "# 目录内部端点（活动通知受众）：空令牌恒 403"),
            ("cmd", "curl :18990/internal/games/1/reservation-users \\"),
            ("out", "    -H 'X-Internal-Token: gif-internal-token'"),
            ("err", f"HTTP {code}  {clip(body, 64)}"),
        ]
    assert_eq(code, 200, "game-catalog internal on")
    return [
        ("comment", "# Internal.Token 配置后重启：受众查询放行"),
        ("cmd", "curl :18990/internal/games/1/reservation-users \\"),
        ("out", "    -H 'X-Internal-Token: gif-internal-token'"),
        ("ok", f"HTTP {code}  {clip(body, 64)}"),
    ]


def demo_realtime_internal_token(state, values, base):
    body_req = '{"user_ids":[42],"event":{"type":"demo","payload":{"msg":"hello"}}}'
    headers = {"X-Internal-Token": INTERNAL_TOKEN}
    code, body = http_call("POST", f"{base}/internal/publish", body_req, headers)
    if state == "off":
        assert_eq(code, 403, "realtime internal off")
        return [
            ("comment", "# 实时投递口令未配置：/internal/publish 恒 403"),
            ("cmd", "curl -X POST :18993/internal/publish \\"),
            ("out", "    -H 'X-Internal-Token: gif-internal-token' \\"),
            ("out", "    -d '{\"user_ids\":[42],\"event\":{\"type\":\"demo\",...}}'"),
            ("err", f"HTTP {code}  {clip(body, 64)}"),
        ]
    assert_eq(code, 200, "realtime internal on")
    return [
        ("comment", "# InternalToken 配置后重启：投递链路放行（用户 42 不在线 delivered=0）"),
        ("cmd", "curl -X POST :18993/internal/publish \\"),
        ("out", "    -H 'X-Internal-Token: gif-internal-token' \\"),
        ("out", "    -d '{\"user_ids\":[42],\"event\":{\"type\":\"demo\",...}}'"),
        ("ok", f"HTTP {code}  {clip(body, 64)}"),
    ]


def demo_realtime_push_webhook(state, values, base):
    """内置 webhook 接收器：off=零投递不回调；on=收到离线推送 POST。"""
    received = []

    class Receiver(http.server.BaseHTTPRequestHandler):
        def do_POST(self):
            length = int(self.headers.get("Content-Length", 0))
            payload = self.rfile.read(length).decode("utf-8", "replace")
            received.append(payload)
            self.send_response(200)
            self.end_headers()

        def log_message(self, *args):
            pass

    receiver = http.server.ThreadingHTTPServer(("127.0.0.1", WEBHOOK_PORT), Receiver)
    threading.Thread(target=receiver.serve_forever, daemon=True).start()
    try:
        body_req = '{"user_ids":[42],"event":{"type":"demo","payload":{"msg":"offline?"}}}'
        headers = {"X-Internal-Token": INTERNAL_TOKEN}
        code, body = http_call("POST", f"{base}/internal/publish", body_req, headers)
        assert_eq(code, 200, "push-webhook publish")
        time.sleep(0.6)  # webhook 异步 fire-and-forget
        if state == "off":
            assert_eq(len(received), 0, "push-webhook off no callback")
            return [
                ("comment", "# 用户 42 无在线连接（delivered=0）；PushWebhook 空"),
                ("cmd", "curl -X POST :18993/internal/publish -d '{\"user_ids\":[42],...}'"),
                ("ok", f"HTTP {code}  {clip(body, 64)}"),
                ("muted", "receiver: (no callbacks)"),
                ("comment", "# 空 PushWebhook = 不推，离线用户静默"),
            ]
        assert_eq(len(received), 1, "push-webhook on callback")
        payload = received[0]
        if len(payload) > 58:
            payload = payload[:58] + "…"
        return [
            ("comment", "# PushWebhook: http://127.0.0.1:18997/push 后重启"),
            ("cmd", "curl -X POST :18993/internal/publish -d '{\"user_ids\":[42],...}'"),
            ("ok", f"HTTP {code}  {clip(body, 64)}"),
            ("muted", "receiver: (waiting…)"),
            ("ok", f"receiver: POST /push {payload}"),
            ("comment", "# 零投递触发离线推送回调（fire-and-forget）"),
        ]
    finally:
        receiver.shutdown()


DEMOS = {
    "community-blocked-words": demo_community_blocked_words,
    "community-moderation-admin-ids": demo_community_admin_ids,
    "community-internal-token": demo_community_internal_token,
    "data-panel-internal-token": demo_data_panel_internal_token,
    "data-panel-analytics-admin-ids": demo_data_panel_analytics_admin,
    "game-catalog-internal-token": demo_game_catalog_internal_token,
    "realtime-hub-internal-token": demo_realtime_internal_token,
    "realtime-hub-push-webhook": demo_realtime_push_webhook,
}

# 帧间的「拨开关」分隔文案（诚实标注配置动作：改配置 + 重启）。
DIVIDERS = {
    "community-blocked-words": 'Moderation.BlockedWords: [] → ["违禁演示词"]  # 重启生效',
    "community-moderation-admin-ids": "Moderation.AdminUserIds: [1] → [1, 7]  # 重启生效",
    "community-internal-token": 'Internal.Token: "" → "gif-internal-token"  # 重启生效',
    "data-panel-internal-token": 'Internal.Token: "" → "gif-internal-token"  # 重启生效',
    "data-panel-analytics-admin-ids": 'Analytics.AdminUserIds: [] → ["7"]  # 重启生效',
    "game-catalog-internal-token": 'Internal.Token: "" → "gif-internal-token"  # 重启生效',
    "realtime-hub-internal-token": 'InternalToken: "" → "gif-internal-token"  # 重启生效',
    "realtime-hub-push-webhook": "PushWebhook: (empty) → http://127.0.0.1:18997/push  # 重启生效",
}


# ---------- 终端帧渲染 ----------

def load_font():
    for path in FONT_CANDIDATES:
        if Path(path).exists():
            return ImageFont.truetype(path, FONT_SIZE)
    die("no CJK font found (tried Noto Sans CJK); install fonts-noto-cjk")


def render_frame(font, title, lines, visible, state_label):
    img = Image.new("RGB", (FRAME_W, FRAME_H), BG)
    draw = ImageDraw.Draw(img)
    draw.rectangle([0, 0, FRAME_W, 34], fill=BAR_BG)
    draw.ellipse([12, 11, 24, 23], fill=(237, 106, 94))
    draw.ellipse([32, 11, 44, 23], fill=(245, 191, 79))
    draw.ellipse([52, 11, 64, 23], fill=(97, 197, 84))
    draw.text((80, 6), f"spawn · {title} · {state_label}", font=font, fill=FG)
    y = MARGIN_Y
    for kind, text in lines[:visible]:
        color = {
            "cmd": PROMPT, "out": FG, "ok": OK_GREEN, "err": ERR_RED,
            "comment": MUTED, "muted": MUTED,
        }.get(kind, FG)
        prefix = "$ " if kind == "cmd" else "  " if kind == "out" else "» " if kind == "comment" else "  "
        draw.text((MARGIN_X, y), prefix + text, font=font, fill=color)
        y += LINE_H
    if visible < len(lines):
        draw.text((MARGIN_X, y), "█", font=font, fill=FG)
    return img


def render_divider(font, title, divider_text):
    img = render_frame(font, title, [("comment", divider_text)], 1, "switching")
    return img


def transcript_to_frames(font, title, lines, state_label):
    return [render_frame(font, title, lines, i + 1, state_label) for i in range(len(lines))]


# ---------- GIF 缝合 ----------

def stitch_gif(frames, gif_path):
    """ffmpeg palette 两段式：concat 列表（帧重复控时）→ 循环 GIF。"""
    tmp = Path(tempfile.mkdtemp(prefix="gif-stitch-"))
    try:
        list_path = tmp / "list.txt"
        with open(list_path, "w") as f:
            f.write("ffconcat version 1.0\n")
            for i, frame in enumerate(frames):
                p = tmp / f"f{i:03d}.png"
                frame.save(p)
                f.write(f"file '{p.name}'\nduration 0.55\n")
            # concat 的 duration 在末帧会被忽略，末帧重复一次兜底
            f.write(f"file 'f{len(frames)-1:03d}.png'\n")
        cmd = [
            "ffmpeg", "-y", "-f", "concat", "-safe", "0", "-i", str(list_path),
            "-vf", "fps=6,split[s0][s1];[s0]palettegen=max_colors=64[p];[s1][p]paletteuse=dither=bayer",
            "-loop", "0", str(gif_path),
        ]
        subprocess.run(cmd, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


# ---------- 单开关流水 ----------

def capture_switch(entry, svc_meta, binaries, font):
    key = entry["key"]
    title = entry["name"]
    if key not in DEMOS:
        die(f"switch {key} marked capture=true but no demo wired in generate.py")
    if key not in STATE_VALUES:
        die(f"switch {key} missing STATE_VALUES")
    demo = DEMOS[key]
    base = f"http://127.0.0.1:{svc_meta['port']}"

    print(f"[capture] {key}: off")
    off_lines = run_state({"meta": svc_meta}, key, "off", STATE_VALUES[key]["off"], lambda s, v: demo("off", v, base))
    print(f"[capture] {key}: on")
    on_lines = run_state({"meta": svc_meta}, key, "on", STATE_VALUES[key]["on"], lambda s, v: demo("on", v, base))

    off_frames = transcript_to_frames(font, title, off_lines, "OFF")
    on_frames = transcript_to_frames(font, title, on_lines, "ON")
    divider = render_divider(font, title, DIVIDERS[key])

    # 帧差闸：off/on 末帧必须不同（几乎一致 = 效果未生效，宁可不产图）
    if off_frames[-1].tobytes() == on_frames[-1].tobytes():
        die(f"{key}: off/on final frames identical — switch effect not visible")

    gif_frames = off_frames[:-1] + [off_frames[-1], off_frames[-1]]
    gif_frames += [divider]
    gif_frames += on_frames[:-1] + [on_frames[-1], on_frames[-1], on_frames[-1]]

    OUT.mkdir(parents=True, exist_ok=True)
    off_path = OUT / f"{key}-off.png"
    on_path = OUT / f"{key}-on.png"
    gif_path = OUT / f"{key}.gif"
    off_frames[-1].save(off_path)
    on_frames[-1].save(on_path)
    stitch_gif(gif_frames, gif_path)
    print(f"[capture] {key}: wrote {off_path.name} {on_path.name} {gif_path.name}")
    return {"key": key, "off": str(off_path.relative_to(ROOT)), "on": str(on_path.relative_to(ROOT)),
            "gif": str(gif_path.relative_to(ROOT))}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--list", action="store_true")
    ap.add_argument("--switch", default="")
    ap.add_argument("--all", action="store_true")
    ap.add_argument("--check", action="store_true")
    args = ap.parse_args()

    data = load_switches()
    switches = data["switches"]
    services = data["services"]

    if args.list:
        for s in switches:
            mark = "capture" if s["capture"] else "manual"
            print(f"{s['key']:40s} [{mark}] {s['config']}")
        return

    if args.check:
        check(data)
        return

    targets = switches if args.all else (
        [s for s in switches if s["key"] == args.switch] if args.switch else []
    )
    if not targets:
        die("nothing to do: pass --all or --switch=<key> (see --list)")

    WORK.mkdir(parents=True, exist_ok=True)
    font = load_font()

    manifest = []
    if MANIFEST.exists():
        manifest = json.loads(MANIFEST.read_text(encoding="utf-8"))
        manifest = [m for m in manifest if m["key"] not in {t["key"] for t in targets}]

    binaries = {}
    for s in targets:
        if not s["capture"]:
            continue
        svc_meta = dict(services[s["service"]])
        svc_meta["service"] = s["service"]
        if s["service"] not in binaries:
            binaries[s["service"]] = build_binary({"meta": svc_meta})
        svc_meta["binary_path"] = binaries[s["service"]]
        # run_state 用 WORK/bin/<binary>，这里只需 dir/port
        manifest.append(capture_switch(s, svc_meta, binaries, font))

    manifest.sort(key=lambda m: m["key"])
    MANIFEST.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"[manifest] {MANIFEST} ({len(manifest)} entries)")


def check(data):
    """一致性闸：capture 开关 ⇔ 产物 ⇔ 清单 ⇔ 文档页全覆盖；不编造不缺漏。"""
    switches = {s["key"]: s for s in data["switches"]}
    capture_keys = {k for k, s in switches.items() if s["capture"]}
    problems = []

    if MANIFEST.exists():
        manifest = {m["key"]: m for m in json.loads(MANIFEST.read_text(encoding="utf-8"))}
    else:
        manifest = {}

    if set(manifest) != capture_keys:
        problems.append(f"manifest keys != capture keys: {set(manifest) ^ capture_keys}")

    for key in capture_keys:
        m = manifest.get(key, {})
        for field in ("off", "on", "gif"):
            path = ROOT / m.get(field, "missing")
            if not path.exists():
                problems.append(f"{key}: missing artifact {field} -> {path}")
        off = manifest.get(key, {}).get("off")
        on = manifest.get(key, {}).get("on")
        if off and on and (ROOT / off).read_bytes() == (ROOT / on).read_bytes():
            problems.append(f"{key}: off/on artifacts identical")

    for key, s in switches.items():
        if not s["capture"] and key in manifest:
            problems.append(f"{key}: capture=false must not have artifacts (不编造)")

    for page in ("docs/guide/config-effects.md", "docs/zh/guide/config-effects.md"):
        p = ROOT / page
        if not p.exists():
            problems.append(f"missing docs page {page}")
            continue
        text = p.read_text(encoding="utf-8")
        for key in switches:
            if f"switch: {key}" not in text:
                problems.append(f"{page}: switch {key} not covered")

    if problems:
        for p in problems:
            print(f"CHECK FAIL: {p}")
        sys.exit(1)
    print(f"check ok: {len(capture_keys)} captured, {len(switches) - len(capture_keys)} manual, docs covered")


if __name__ == "__main__":
    main()
