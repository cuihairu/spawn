package handler

// GP-401 游戏评价（F04）+ 收藏与上线通知（F05）集成测试：
//   - 评分：1-5 覆盖式，越界 400；聚合随详情/评分响应回显；未登录 401
//   - 长评：一游戏一评（重复 409）、长度口径 400、作者改评、非作者 404、
//     公开列表带作者评分、未上架游戏 404
//   - 举报：幂等（重复返回既有）、管理队列、dismiss（评价保留）/remove
//     （评价删除 + 同评其余未决结案）、已处置 409
//   - 开发者回复：合作方自有游戏 200 + 审计，无合作方绑定 403，
//     他人合作方 403
//   - 收藏：幂等收藏/取消、我的收藏；上线通知（game_launch）只在上架过渡
//     向收藏人群扇出一次；未上架不可收藏 404

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/utils"
)

// setupGP401 造一款已上架游戏 + 合作方归属（开发者回复前置）。
// 返回 (gameID, partner 令牌, 他人合作方令牌)。
func setupGP401(t *testing.T, base string, sign func(utils.JWTClaims) string) (string, string, string) {
	t.Helper()
	admin := sign(claimsFor(1, "admin"))
	gameBody := `{"title":"评价游戏GP401","description":"gp-401 游戏评价与收藏","genres":["RPG"],"platforms":["Android"],"release_date":"2026-11-01","developer":"d","publisher":"p","tags":[],"score":0,"cover_image":"https://example.com/cover.jpg"}`
	code, body := doAuthed(t, "POST", base+"/games", admin, gameBody)
	if code != 200 {
		t.Fatalf("create game = %d body=%s", code, body)
	}
	var gameResp struct {
		Game struct {
			Id string `json:"id"`
		} `json:"game"`
	}
	if err := json.Unmarshal([]byte(body), &gameResp); err != nil || gameResp.Game.Id == "" {
		t.Fatalf("decode game: %v body=%s", err, body)
	}
	gameID := gameResp.Game.Id

	partnerUserID := int64(101)
	code, body = doAuthed(t, "POST", base+"/admin/partners", admin, `{"name":"评价厂GP401"}`)
	if code != 200 {
		t.Fatalf("create partner = %d body=%s", code, body)
	}
	var partnerResp struct {
		Id int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &partnerResp)
	partnerIDStr := strconv.FormatInt(partnerResp.Id, 10)
	if code, _ = doAuthed(t, "POST", base+"/admin/roles", admin,
		`{"user_id":101,"role":"partner","partner_id":`+partnerIDStr+`}`); code != 200 {
		t.Fatalf("grant partner = %d", code)
	}
	if code, _ = doAuthed(t, "POST", base+"/admin/games/"+gameID+"/ownership", admin,
		`{"partner_id":`+partnerIDStr+`}`); code != 200 {
		t.Fatalf("assign ownership = %d", code)
	}

	// 他人合作方（无该游戏归属）：开发者回复 403 用例
	code, body = doAuthed(t, "POST", base+"/admin/partners", admin, `{"name":"无关厂GP401"}`)
	if code != 200 {
		t.Fatalf("create other partner = %d", code)
	}
	var otherPartnerResp struct {
		Id int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &otherPartnerResp); err != nil || otherPartnerResp.Id == 0 {
		t.Fatalf("decode other partner: %v body=%s", err, body)
	}
	otherIDStr := strconv.FormatInt(otherPartnerResp.Id, 10)
	if code, _ = doAuthed(t, "POST", base+"/admin/roles", admin,
		`{"user_id":102,"role":"partner","partner_id":`+otherIDStr+`}`); code != 200 {
		t.Fatalf("grant other partner = %d", code)
	}
	return gameID, sign(claimsFor(partnerUserID, "partner-gp401")), sign(claimsFor(102, "partner-other"))
}

func TestRoutes_GameRatingsAndReviews(t *testing.T) {
	stub := newCommunityStub()
	defer stub.srv.Close()
	base, sign := newReservationTestServer(t, stub.srv.URL)
	gameID, partner, otherPartner := setupGP401(t, base, sign)
	admin := sign(claimsFor(1, "admin"))
	player7 := sign(claimsFor(7, "bob"))
	player8 := sign(claimsFor(8, "carol"))
	player9 := sign(claimsFor(9, "dave"))

	// 1) 评分：覆盖式 + 聚合随响应回显
	code, body := doAuthed(t, "POST", base+"/games/"+gameID+"/rating", player7, `{"rating":5}`)
	if code != 200 || !strings.Contains(body, `"rating":5`) || !strings.Contains(body, `"average":5`) ||
		!strings.Contains(body, `"count":1`) {
		t.Fatalf("rate = %d body=%s", code, body)
	}
	code, body = doAuthed(t, "POST", base+"/games/"+gameID+"/rating", player7, `{"rating":4}`)
	if code != 200 || !strings.Contains(body, `"average":4`) || !strings.Contains(body, `"count":1`) {
		t.Fatalf("re-rate = %d body=%s", code, body)
	}
	if code, _ = doAuthed(t, "POST", base+"/games/"+gameID+"/rating", player8, `{"rating":4}`); code != 200 {
		t.Fatalf("rate2 = %d", code)
	}
	// 越界与未登录
	if code, _ = doAuthed(t, "POST", base+"/games/"+gameID+"/rating", player7, `{"rating":0}`); code != 400 {
		t.Fatalf("rating 0 = %d, want 400", code)
	}
	if code, _ = doAuthed(t, "POST", base+"/games/"+gameID+"/rating", player7, `{"rating":6}`); code != 400 {
		t.Fatalf("rating 6 = %d, want 400", code)
	}
	if code, _ = doAuthed(t, "POST", base+"/games/"+gameID+"/rating", "", `{"rating":3}`); code != 401 {
		t.Fatalf("anonymous rate = %d, want 401", code)
	}

	// 2) 详情附评分聚合（GP-401 公开读口径）
	code, body = doAuthed(t, "GET", base+"/games/"+gameID, "", "")
	if code != 200 || !strings.Contains(body, `"count":2`) || !strings.Contains(body, `"average":4`) {
		t.Fatalf("detail rating = %d body=%s", code, body)
	}

	// 3) 长评：一游戏一评、长度口径、公开列表带作者评分
	code, body = doAuthed(t, "POST", base+"/games/"+gameID+"/reviews", player7, `{"content":"短"}`)
	if code != 400 {
		t.Fatalf("short review = %d, want 400", code)
	}
	code, body = doAuthed(t, "POST", base+"/games/"+gameID+"/reviews", player7, `{"content":"  画面和手感都在线，剧情后半段更精彩。  "}`)
	if code != 200 || !strings.Contains(body, `"dev_reply":""`) {
		t.Fatalf("create review = %d body=%s", code, body)
	}
	var review7Resp struct {
		Id int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &review7Resp); err != nil || review7Resp.Id == 0 {
		t.Fatalf("decode review7: %v body=%s", err, body)
	}
	review7ID := strconv.FormatInt(review7Resp.Id, 10)
	code, body = doAuthed(t, "POST", base+"/games/"+gameID+"/reviews", player7, `{"content":"同一玩家重复提交应当被唯一约束拒绝。"}`)
	if code != 409 {
		t.Fatalf("duplicate review = %d body=%s, want 409", code, body)
	}
	code, body = doAuthed(t, "POST", base+"/games/"+gameID+"/reviews", player8, `{"content":"后期内容略肝，但前期体验不错。"}`)
	if code != 200 {
		t.Fatalf("create review2 = %d body=%s", code, body)
	}
	var review8Resp struct {
		Id int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &review8Resp); err != nil || review8Resp.Id == 0 {
		t.Fatalf("decode review8: %v body=%s", err, body)
	}
	review8ID := strconv.FormatInt(review8Resp.Id, 10)
	code, body = doAuthed(t, "POST", base+"/games/"+gameID+"/reviews", player9, `{"content":"第九位玩家的长评，用于验证移除后列表收缩。"}`)
	if code != 200 {
		t.Fatalf("create review3 = %d body=%s", code, body)
	}

	code, body = doAuthed(t, "GET", base+"/games/"+gameID+"/reviews", "", "")
	if code != 200 || !strings.Contains(body, `"total":3`) ||
		!strings.Contains(body, `"rating":4`) || !strings.Contains(body, `"user_id":8`) {
		t.Fatalf("list reviews = %d body=%s", code, body)
	}

	// 4) 改评：作者 200，非作者按不存在 404
	code, body = doAuthed(t, "PUT", base+"/games/"+gameID+"/reviews/"+review7ID, player7, `{"content":"通关后补充：剧情后半段更精彩，值得入手。"}`)
	if code != 200 || !strings.Contains(body, `"rating":4`) {
		t.Fatalf("update own review = %d body=%s", code, body)
	}
	if code, _ = doAuthed(t, "PUT", base+"/games/"+gameID+"/reviews/"+review8ID, player7, `{"content":"替他人改评应当按不存在处理。"}`); code != 404 {
		t.Fatalf("update other review = %d, want 404", code)
	}

	// 5) 举报：幂等 + 管理处置 dismiss（评价保留）/remove（评价删除 + 同评结案）
	code, body = doAuthed(t, "POST", base+"/games/"+gameID+"/reviews/"+review8ID+"/report", player7, `{"reason":"疑似抄袭"}`)
	if code != 200 || !strings.Contains(body, `"created":true`) {
		t.Fatalf("report = %d body=%s", code, body)
	}
	var reportResp struct {
		Id int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &reportResp); err != nil || reportResp.Id == 0 {
		t.Fatalf("decode report: %v body=%s", err, body)
	}
	reportA := strconv.FormatInt(reportResp.Id, 10)
	code, body = doAuthed(t, "POST", base+"/games/"+gameID+"/reviews/"+review8ID+"/report", player7, `{"reason":"再次举报"}`)
	if code != 200 || !strings.Contains(body, `"created":false`) || !strings.Contains(body, `"id":`+reportA) {
		t.Fatalf("report replay = %d body=%s", code, body)
	}
	// 空理由 400；他人评价不存在 404
	if code, _ = doAuthed(t, "POST", base+"/games/"+gameID+"/reviews/"+review8ID+"/report", player7, `{"reason":""}`); code != 400 {
		t.Fatalf("empty reason = %d, want 400", code)
	}
	if code, _ = doAuthed(t, "POST", base+"/games/"+gameID+"/reviews/999999/report", player7, `{"reason":"举报不存在的评"}`); code != 404 {
		t.Fatalf("report missing review = %d, want 404", code)
	}
	// 第二位举报人各一条
	if code, _ = doAuthed(t, "POST", base+"/games/"+gameID+"/reviews/"+review8ID+"/report", player8, `{"reason":"低质水文"}`); code != 200 {
		t.Fatalf("second reporter = %d", code)
	}
	code, body = doAuthed(t, "GET", base+"/admin/review-reports?status=pending", admin, "")
	if code != 200 || !strings.Contains(body, `"total":2`) {
		t.Fatalf("pending reports = %d body=%s", code, body)
	}
	// 玩家不能处置举报
	if code, _ = doAuthed(t, "POST", base+"/admin/review-reports/"+reportA+"/handle", player7, `{"action":"dismiss"}`); code != 403 {
		t.Fatalf("player handle = %d, want 403", code)
	}
	// dismiss：评价保留
	code, body = doAuthed(t, "POST", base+"/admin/review-reports/"+reportA+"/handle", admin, `{"action":"dismiss"}`)
	if code != 200 || !strings.Contains(body, `"status":"dismissed"`) {
		t.Fatalf("dismiss = %d body=%s", code, body)
	}
	if code, _ = doAuthed(t, "POST", base+"/admin/review-reports/"+reportA+"/handle", admin, `{"action":"dismiss"}`); code != 409 {
		t.Fatalf("re-handle = %d, want 409", code)
	}
	code, body = doAuthed(t, "GET", base+"/games/"+gameID+"/reviews", "", "")
	if code != 200 || !strings.Contains(body, `"total":3`) {
		t.Fatalf("reviews after dismiss = %d body=%s", code, body)
	}
	// 处置第二条为 remove：评价删除 + 未决队列清空
	code, body = doAuthed(t, "GET", base+"/admin/review-reports?status=pending", admin, "")
	if code != 200 {
		t.Fatalf("pending list = %d body=%s", code, body)
	}
	var pendingResp struct {
		Items []struct {
			Id int64 `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &pendingResp); err != nil || len(pendingResp.Items) != 1 {
		t.Fatalf("pending items = %+v err=%v", pendingResp, err)
	}
	reportB := strconv.FormatInt(pendingResp.Items[0].Id, 10)
	code, body = doAuthed(t, "POST", base+"/admin/review-reports/"+reportB+"/handle", admin, `{"action":"remove"}`)
	if code != 200 || !strings.Contains(body, `"status":"resolved"`) {
		t.Fatalf("remove = %d body=%s", code, body)
	}
	code, body = doAuthed(t, "GET", base+"/games/"+gameID+"/reviews", "", "")
	if code != 200 || !strings.Contains(body, `"total":2`) {
		t.Fatalf("reviews after remove = %d body=%s", code, body)
	}
	code, body = doAuthed(t, "GET", base+"/admin/review-reports?status=pending", admin, "")
	if code != 200 || !strings.Contains(body, `"total":0`) {
		t.Fatalf("pending after remove = %d body=%s", code, body)
	}
	// 非法动作 400
	if code, _ = doAuthed(t, "POST", base+"/admin/review-reports/"+reportA+"/handle", admin, `{"action":"ban"}`); code != 400 {
		t.Fatalf("bad action = %d, want 400", code)
	}

	// 6) 开发者回复：自有游戏 200（列表可见）+ 审计；无关合作方 403；
	//    无合作方绑定（player）403
	code, body = doAuthed(t, "POST", base+"/partner/games/"+gameID+"/reviews/"+review7ID+"/reply", partner, `{"content":"感谢反馈，剧情优化在下一版本"}`)
	if code != 200 || !strings.Contains(body, `"dev_reply":"感谢反馈，剧情优化在下一版本"`) {
		t.Fatalf("dev reply = %d body=%s", code, body)
	}
	code, body = doAuthed(t, "GET", base+"/games/"+gameID+"/reviews", "", "")
	if code != 200 || !strings.Contains(body, `"dev_reply":"感谢反馈，剧情优化在下一版本"`) {
		t.Fatalf("list with dev reply = %d body=%s", code, body)
	}
	if code, _ = doAuthed(t, "POST", base+"/partner/games/"+gameID+"/reviews/"+review7ID+"/reply", otherPartner, `{"content":"不属于本合作方的游戏应当 403"}`); code != 403 {
		t.Fatalf("other partner reply = %d, want 403", code)
	}
	if code, _ = doAuthed(t, "POST", base+"/partner/games/"+gameID+"/reviews/"+review7ID+"/reply", player7, `{"content":"玩家不能回复"}`); code != 403 {
		t.Fatalf("player reply = %d, want 403", code)
	}
	if code, _ = doAuthed(t, "POST", base+"/partner/games/"+gameID+"/reviews/"+review7ID+"/reply", partner, `{"content":""}`); code != 400 {
		t.Fatalf("empty reply = %d, want 400", code)
	}
	code, body = doAuthed(t, "GET", base+"/admin/audit?resource_type=game_user_review", admin, "")
	if code != 200 || !strings.Contains(body, `"review_dev_reply"`) {
		t.Fatalf("audit dev reply = %d body=%s", code, body)
	}

	// 7) 收藏：幂等 + 我的收藏 + 未上架不可收藏 + 上线通知只发一次
	code, body = doAuthed(t, "POST", base+"/games/"+gameID+"/favorite", player7, "")
	if code != 200 || !strings.Contains(body, `"created":true`) {
		t.Fatalf("favorite = %d body=%s", code, body)
	}
	code, body = doAuthed(t, "POST", base+"/games/"+gameID+"/favorite", player7, "")
	if code != 200 || !strings.Contains(body, `"created":false`) {
		t.Fatalf("favorite replay = %d body=%s", code, body)
	}
	code, body = doAuthed(t, "GET", base+"/me/favorites", player7, "")
	if code != 200 || !strings.Contains(body, `"title":"评价游戏GP401"`) {
		t.Fatalf("my favorites = %d body=%s", code, body)
	}
	// 未上架不可收藏（404 不泄露存在性）
	if code, _ = doAuthed(t, "POST", base+"/admin/games/"+gameID+"/unpublish", admin, ""); code != 200 {
		t.Fatalf("unpublish = %d", code)
	}
	if code, _ = doAuthed(t, "POST", base+"/games/"+gameID+"/favorite", player8, ""); code != 404 {
		t.Fatalf("favorite offline = %d, want 404", code)
	}
	// 上架过渡 → game_launch 只向收藏人群扇出一次
	if code, _ = doAuthed(t, "POST", base+"/admin/games/"+gameID+"/publish", admin, ""); code != 200 {
		t.Fatalf("publish = %d", code)
	}
	if stub.Calls() != 1 {
		t.Fatalf("launch should notify once, got %d calls", stub.Calls())
	}
	raw, token := stub.Last()
	var fanout struct {
		UserIds []int64 `json:"user_ids"`
		Type    string  `json:"type"`
	}
	if err := json.Unmarshal(raw, &fanout); err != nil || token != gp306Token || fanout.Type != "game_launch" ||
		len(fanout.UserIds) != 1 || fanout.UserIds[0] != 7 {
		t.Fatalf("launch fanout = %s type=%s err=%v", raw, fanout.Type, err)
	}
	// 再次上下架：无收藏人群变化即不重发（仅校验通知未因重复上架累加）
	if code, _ = doAuthed(t, "POST", base+"/admin/games/"+gameID+"/unpublish", admin, ""); code != 200 {
		t.Fatalf("unpublish2 = %d", code)
	}
	if code, _ = doAuthed(t, "POST", base+"/admin/games/"+gameID+"/publish", admin, ""); code != 200 {
		t.Fatalf("publish2 = %d", code)
	}
	if stub.Calls() != 2 {
		t.Fatalf("second launch should notify once more, got %d calls", stub.Calls())
	}
	// 取消收藏（幂等）后我的收藏清空
	code, _ = doAuthed(t, "DELETE", base+"/games/"+gameID+"/favorite", player7, "")
	if code != 200 {
		t.Fatalf("unfavorite = %d", code)
	}
	code, _ = doAuthed(t, "DELETE", base+"/games/"+gameID+"/favorite", player7, "")
	if code != 200 {
		t.Fatalf("unfavorite replay = %d", code)
	}
	code, body = doAuthed(t, "GET", base+"/me/favorites", player7, "")
	if code != 200 || !strings.Contains(body, `"items":[]`) {
		t.Fatalf("my favorites after unfavorite = %d body=%s", code, body)
	}
	// 未登录收藏 401
	if code, _ = doAuthed(t, "POST", base+"/games/"+gameID+"/favorite", "", ""); code != 401 {
		t.Fatalf("anonymous favorite = %d, want 401", code)
	}
}
