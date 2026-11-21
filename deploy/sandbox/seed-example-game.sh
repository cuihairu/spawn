#!/usr/bin/env bash
# GP-005 联合沙箱：一款示例游戏的端到端装配（示例游戏 = 沙箱验收最小资产集）。
#
# 链路：注册管理员/合作方账号 → 建游戏 → 指派归属 → 合作方提报版本 → 平台审核
# 通过 → 发布游戏 → 发布版本 → 建区服 → Courier 映射与能力 → 支付商品 →
# 礼包（全量上限）→ 活动（倍率赠送）→ 玩家侧冒烟（活动列表/预约/礼包领取）。
#
# 管理面走各服务业务端口直连：POST /games 等管理面路由不经 api-gateway 代理
# （网关只反代玩家侧与只读运营面），沙箱引导工具不走网关。
#
# 前置：docker compose up -d（见 deploy/sandbox/README.md）；game-catalog 的
# ADMIN_USER_IDS 须含本脚本管理员账号的 user-service 用户 id（启动期幂等写为
# platform_admin）。全新沙箱 ADMIN_USER_IDS=1 时，管理员账号须为第一个注册者。
#
# 用法（Linux/bash，依赖 curl + jq）：
#   SP_ADMIN_USERNAME=sandbox-admin ./deploy/sandbox/seed-example-game.sh
#   ./deploy/sandbox/seed-example-game.sh
#
# 环境变量：SP_GATEWAY/SP_GAME_CATALOG/SP_COURIER_ADAPTER/SP_PAYMENTS/
# SP_PROMOTIONS（服务地址，默认 127.0.0.1 业务端口）、SP_ADMIN_USERNAME、
# SP_PARTNER_USERNAME、SP_PLAYER_USERNAME、SP_PASSWORD、SP_COURIER_GAME_ID、
# SEED_TAG（资产 id 带标签；重复装配请换新标签——商品主键是插入式写入，同标签
# 重跑会在商品步撞主键 409）。

set -euo pipefail

USER_URL="${SP_USER:-http://127.0.0.1:8888}"
CATALOG="${SP_GAME_CATALOG:-http://127.0.0.1:8890}"
ADAPTER="${SP_COURIER_ADAPTER:-http://127.0.0.1:8897}"
PAYMENTS="${SP_PAYMENTS:-http://127.0.0.1:8898}"
PROMOTIONS="${SP_PROMOTIONS:-http://127.0.0.1:8902}"

ADMIN_USER="${SP_ADMIN_USERNAME:-sandbox-admin}"
PARTNER_USER="${SP_PARTNER_USERNAME:-sandbox-partner}"
PLAYER_USER="${SP_PLAYER_USERNAME:-sandbox-player}"
PASSWORD="${SP_PASSWORD:-Sandbox!12345678}"
SEED_TAG="${SEED_TAG:-sandbox-$(date +%Y%m%d-%H%M%S)}"
# Courier 映射表 courier_game_id 唯一：每次装配用带标签的独立 id，重跑不撞键。
COURIER_GAME_ID="${SP_COURIER_GAME_ID:-courier-sandbox-$SEED_TAG}"
PRODUCT_ID="p_sandbox_${SEED_TAG}" # 商品主键是路径 :id（payment_products.id），重跑须换

for tool in curl jq date; do
  command -v "$tool" >/dev/null || { echo "缺少依赖：$tool" >&2; exit 2; }
done

# --- HTTP 小工具 -----------------------------------------------------------------

RESP=""

# call METHOD URL TOKEN JSON_BODY [HINT] —— 非 2xx 直接失败并回显响应体；
# 成功时把响应体写到 stdout（供 assert/jq 直接读 stdin），第 5 参为失败提示
# （如 403 指向 ADMIN_USER_IDS 引导）。不用全局变量传 body：管道两侧是不同
# 子壳，全局变量跨管道不可见（会静默得到空响应）。
call() {
  local method="$1" url="$2" token="$3" body="${4:-}" hint="${5:-}"
  local auth=()
  if [ -n "$token" ]; then auth+=(-H "Authorization: Bearer $token"); fi
  if [ -n "$body" ]; then
    RESP="$(curl -sS -m 20 -X "$method" "${auth[@]}" -H 'Content-Type: application/json' \
      -d "$body" -w '\n%{http_code}' "$url")" || exit 1
  else
    RESP="$(curl -sS -m 20 -X "$method" "${auth[@]}" -w '\n%{http_code}' "$url")" || exit 1
  fi
  local code="${RESP##*$'\n'}"
  RESP="${RESP%$'\n'*}"
  case "$code" in
    2*) printf '%s' "$RESP" ;;
    *) echo "[FAIL] $method $url -> HTTP $code $RESP" >&2
       if [ -n "$hint" ]; then echo "       $hint" >&2; fi
       exit 1 ;;
  esac
}

# 断言 stdin 上的 JSON 满足 jq 表达式（取值为 null 或空视为失败）。
assert() {
  local label="$1" expr="$2"
  local body v
  body="$(cat)"
  v="$(jq -r "$expr" <<<"$body")"
  if [ -z "$v" ] || [ "$v" = "null" ]; then
    echo "[FAIL] $label：期望 $expr 有值，响应=$body" >&2
    exit 1
  fi
  echo "$v"
}

step() { echo "[${1:-}] $2"; }

# 先注册（已存在则忽略失败）再登录，返回 JWT。
# 不用「登录失败才注册」：user-service 密码错返 400 而非 401/404，
# 按状态码判断会漏注册、把「账号不存在」误报成密码错误。
auth() {
  local user="$1"
  local resp code
  curl -sS -m 20 -o /dev/null -X POST "$USER_URL/auth/register" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"$user\",\"email\":\"$user@sandbox.com\",\"password\":\"$PASSWORD\"}" || true
  resp="$(curl -sS -m 20 -w '\n%{http_code}' -X POST "$USER_URL/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"$user\",\"password\":\"$PASSWORD\"}")"
  code="${resp##*$'\n'}"
  resp="${resp%$'\n'*}"
  if [ "${code:0:1}" != "2" ]; then
    echo "[FAIL] 账号 $user 登录失败：HTTP $code $resp" >&2
    echo "       邮箱须以 .com/.cn 结尾（user-service 校验口径），默认取 \$user@sandbox.com" >&2
    exit 1
  fi
  jq -r '.token' <<<"$resp"
}

# --- 1. 管理员与角色 --------------------------------------------------------------

step 1 "管理员账号 $ADMIN_USER 登录（管理员身份由首个管理面调用验证）"
ADMIN_TOKEN="$(auth "$ADMIN_USER")"

# --- 2. 合作方与示例游戏 ----------------------------------------------------------

ADMIN_HINT="HTTP 403 = 该账号未被 game-catalog 的 ADMIN_USER_IDS 覆盖。查 id：登录响应 user_info.id；写入后 ADMIN_USER_IDS=<id> docker compose up -d game-catalog 重启生效"
step 2 "创建合作方（此调用即管理员身份预检）并授权合作方账号"
PARTNER_ID="$(call POST "$CATALOG/admin/partners" "$ADMIN_TOKEN" \
  "{\"name\":\"Sandbox Partner $SEED_TAG\",\"contact_email\":\"partner@sandbox.com\"}" \
  "$ADMIN_HINT" | assert '合作方 id' '.id')"

PARTNER_TOKEN="$(auth "$PARTNER_USER")"
# auth() 只回传 token；partner 的 user id 由登录响应回读
PARTNER_ID_USER="$(curl -sS -m 20 -X POST "$USER_URL/auth/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$PARTNER_USER\",\"password\":\"$PASSWORD\"}" | jq -r '.user_info.id')"
PARTNER_ROLE="$(call POST "$CATALOG/admin/roles" "$ADMIN_TOKEN" \
  "{\"user_id\":$PARTNER_ID_USER,\"role\":\"partner\",\"partner_id\":$PARTNER_ID}" \
  | assert '合作方角色' '.role')"
[ "$PARTNER_ROLE" = "partner" ] || { echo "[FAIL] 合作方角色写入失败：$PARTNER_ROLE" >&2; exit 1; }

GAME_ID="$(call POST "$CATALOG/games" "$ADMIN_TOKEN" \
  "{\"title\":\"示例游戏 $SEED_TAG\",\"description\":\"GP-005 联合沙箱示例游戏\",\"genres\":[\"demo\"],\"platforms\":[\"android\"],\"release_date\":\"2026-01-01\",\"developer\":\"Sandbox Dev\",\"publisher\":\"Sandbox Pub\",\"tags\":[\"联运\"],\"score\":0,\"cover_image\":\"https://example.com/cover.jpg\"}" \
  | assert '游戏 id' '.game.id')"

call POST "$CATALOG/admin/games/$GAME_ID/ownership" "$ADMIN_TOKEN" "{\"partner_id\":$PARTNER_ID}" >/dev/null

# --- 3. 版本提审 → 过审 → 发布 ------------------------------------------------------

VERSION_ID="$(call POST "$CATALOG/partner/games/$GAME_ID/versions" "$PARTNER_TOKEN" \
  '{"version":"1.0.0","changelog":"沙箱首版","package_name":"com.spawn.sandbox.example","version_code":1,"size_bytes":0,"hash":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}' \
  | assert '版本 id' '.id')"

REVIEW_ID="$(call POST "$CATALOG/partner/games/$GAME_ID/submit" "$PARTNER_TOKEN" \
  "{\"version_id\":\"$VERSION_ID\"}" | assert '提审 id' '.id')"

call POST "$CATALOG/admin/reviews/$REVIEW_ID/decision" "$ADMIN_TOKEN" \
  '{"decision":"approve","reason":"沙箱验收通过"}' >/dev/null

call POST "$CATALOG/admin/games/$GAME_ID/publish" "$ADMIN_TOKEN" >/dev/null

call POST "$CATALOG/admin/games/$GAME_ID/versions/$VERSION_ID/publish" "$ADMIN_TOKEN" >/dev/null

LATEST="$(call GET "$CATALOG/games/$GAME_ID/versions/latest" "" | jq -r '.version.version')"
[ "$LATEST" = "1.0.0" ] || { echo "[FAIL] 已发布版本异常：$LATEST" >&2; exit 1; }

# --- 4. 区服 / Courier 映射 / 支付商品 ---------------------------------------------

PLANNED_OPEN_AT="$(date -u -d '+7 days' '+%Y-%m-%dT%H:%M:%SZ')"
# 区服默认建为 open（servermodel 默认值）；示例游戏建 closed+排期，玩家预约
# 才有意义（已开服预约返回 409，属 GP-306 正常口径）。
SERVER_ID="$(call POST "$CATALOG/partner/games/$GAME_ID/servers" "$PARTNER_TOKEN" \
  "{\"name\":\"沙箱一服\",\"region\":\"cn-north\",\"status\":\"closed\",\"planned_open_at\":\"$PLANNED_OPEN_AT\"}" \
  | assert '区服 id' '.id')"

call PUT "$ADAPTER/admin/games/$GAME_ID/mapping" "$ADMIN_TOKEN" \
  "{\"courierGameId\":\"$COURIER_GAME_ID\",\"Env\":\"sandbox\",\"PartnerId\":$PARTNER_ID,\"capabilities\":{\"login\":true,\"payment\":true,\"download\":true,\"support\":true}}" \
  >/dev/null
# 写操作返回空体，须读回核验。读回 env 必须真是 sandbox：UpsertMappingRequest 的
# Env/PartnerId 无显式 JSON 名（json:",default=prod"），小写键会被忽略并落回
# 默认值——静默写成 prod。
MAP_BODY="$(call GET "$ADAPTER/admin/games/$GAME_ID/mapping" "$ADMIN_TOKEN")"
MAPPING="$(jq -r '.spawnGameId' <<<"$MAP_BODY")"
[ "$MAPPING" = "$GAME_ID" ] || { echo "[FAIL] 映射游戏 id 不符：$MAPPING" >&2; exit 1; }
MAP_ENV="$(jq -r '.env' <<<"$MAP_BODY")"
[ "$MAP_ENV" = "sandbox" ] || { echo "[FAIL] 映射 env 读回为 $MAP_ENV（期望 sandbox；Env 须用大写键名）" >&2; exit 1; }

# 商品路径 :id 即主键（409「商品 id 已存在」= 主键撞行，product_id 无唯一索引）
call POST "$PAYMENTS/admin/products/$PRODUCT_ID" "$ADMIN_TOKEN" \
  "{\"product_id\":\"$PRODUCT_ID\",\"form\":\"DIRECT_PURCHASE\",\"amount_cents\":1000,\"base_quantity\":100,\"currency\":\"CNY\",\"game_id\":\"$GAME_ID\",\"partner_id\":$PARTNER_ID}" \
  >/dev/null

# --- 5. 礼包（全量上限）与活动（倍率赠送）----------------------------------------

NOW_START="$(date -u -d '-1 hour' '+%Y-%m-%dT%H:%M:%SZ')"
NOW_END="$(date -u -d '+30 days' '+%Y-%m-%dT%H:%M:%SZ')"

PACK_ID="$(call POST "$PROMOTIONS/admin/gift-packs" "$ADMIN_TOKEN" \
  "{\"name\":\"沙箱包\",\"game_id\":\"$GAME_ID\",\"description\":\"GP-005 示例礼包\",\"timezone\":\"UTC\",\"start_at\":\"$NOW_START\",\"end_at\":\"$NOW_END\",\"server_ids\":[$SERVER_ID],\"channel\":\"\",\"audience\":{\"type\":\"all\"},\"per_user_limit\":2,\"total_limit\":5,\"code_required\":false,\"content\":{\"items\":[{\"sku\":\"gem-100\",\"qty\":1}]}}" \
  | assert '礼包 id' '.gift_pack.id')"
call POST "$PROMOTIONS/admin/gift-packs/$PACK_ID/publish" "$ADMIN_TOKEN" >/dev/null

ACTIVITY_ID="$(call POST "$PROMOTIONS/admin/activities" "$ADMIN_TOKEN" \
  "{\"name\":\"沙箱倍率活动\",\"game_id\":\"$GAME_ID\",\"timezone\":\"UTC\",\"start_at\":\"$NOW_START\",\"end_at\":\"$NOW_END\",\"server_ids\":[$SERVER_ID],\"channel\":\"\",\"audience\":{\"type\":\"all\"},\"trigger\":{\"type\":\"order\",\"min_amount_cents\":1000},\"limits\":{\"per_user_count\":3,\"budget_cents\":100000},\"stackable\":false,\"stack_group\":\"sandbox\",\"rewards\":{\"type\":\"multiplier\",\"mode\":\"total\",\"rate_bp\":15000}}" \
  | assert '活动 id' '.activity.id')"
call POST "$PROMOTIONS/admin/activities/$ACTIVITY_ID/publish" "$ADMIN_TOKEN" >/dev/null

# --- 6. 玩家侧冒烟 -----------------------------------------------------------------

PLAYER_TOKEN="$(auth "$PLAYER_USER")"

ITEMS="$(call GET "$PROMOTIONS/games/$GAME_ID/activities" "" | jq -r '(.items // []) | length')"
[ "$ITEMS" -ge 1 ] || { echo "[FAIL] 玩家侧活动列表为空" >&2; exit 1; }

call POST "$CATALOG/games/$GAME_ID/servers/$SERVER_ID/reserve" "$PLAYER_TOKEN" >/dev/null

CLAIM_OK="$(call POST "$PROMOTIONS/gift-packs/$PACK_ID/claim" "$PLAYER_TOKEN" \
  "{\"server_id\":$SERVER_ID,\"request_id\":\"sandbox-seed-claim\"}" \
  | assert '礼包领取' '.claim.id')"

echo
echo "沙箱示例游戏装配完成"
echo "  game_id       = $GAME_ID"
echo "  version       = $VERSION_ID（1.0.0，已发布）"
echo "  server_id     = $SERVER_ID"
echo "  courier       = $COURIER_GAME_ID（env=sandbox，login/payment/download/support）"
echo "  product       = $PRODUCT_ID（1000 分 / base_quantity 100）"
echo "  gift_pack     = $PACK_ID（per_user 2 / total 5）"
echo "  activity      = $ACTIVITY_ID（倍率 15000 bp，预算 100000）"
echo "  player claim  = $CLAIM_OK"
echo "  标签          = $SEED_TAG"
