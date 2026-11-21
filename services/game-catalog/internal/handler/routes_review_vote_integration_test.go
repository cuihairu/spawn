package handler

// GP-401 长评有用票集成测试：匿名 401、越权 404（不存在/不属该游戏）、
// 一人一票幂等（重复投不重复计数）、取消投票、列表票数回显与 sort=votes
// 排序（默认新在前）。

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestRoutes_GameReviewVote(t *testing.T) {
	stub := newCommunityStub()
	defer stub.srv.Close()
	base, sign, _ := newReservationTestServer(t, stub.srv.URL)
	gameID, _, _ := setupGP401(t, base, sign)
	player7 := sign(claimsFor(7, "bob"))
	player8 := sign(claimsFor(8, "carol"))

	// 两条长评
	code, body := doAuthed(t, "POST", base+"/games/"+gameID+"/reviews", player7, `{"content":"画面和手感都在线，剧情后半段更精彩。"}`)
	if code != 200 {
		t.Fatalf("create review7 = %d body=%s", code, body)
	}
	var r7 struct {
		Id int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &r7)
	code, body = doAuthed(t, "POST", base+"/games/"+gameID+"/reviews", player8, `{"content":"后期内容略肝，但前期体验不错。"}`)
	if code != 200 {
		t.Fatalf("create review8 = %d body=%s", code, body)
	}
	var r8 struct {
		Id int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &r8)

	// 匿名投票 401
	if code, _ = doAuthed(t, "POST", base+"/games/"+gameID+"/reviews/"+strconv.FormatInt(r7.Id, 10)+"/vote", "", ""); code != 401 {
		t.Fatalf("anonymous vote = %d, want 401", code)
	}

	// 投：r7 得 2 票（player7 + player8），player7 重复投幂等
	code, body = doAuthed(t, "POST", base+"/games/"+gameID+"/reviews/"+strconv.FormatInt(r7.Id, 10)+"/vote", player7, "")
	if code != 200 || !strings.Contains(body, `"votes":1`) || !strings.Contains(body, `"voted":true`) {
		t.Fatalf("vote = %d body=%s", code, body)
	}
	code, body = doAuthed(t, "POST", base+"/games/"+gameID+"/reviews/"+strconv.FormatInt(r7.Id, 10)+"/vote", player7, "")
	if code != 200 || !strings.Contains(body, `"votes":1`) {
		t.Fatalf("vote replay = %d body=%s, want votes=1", code, body)
	}
	code, body = doAuthed(t, "POST", base+"/games/"+gameID+"/reviews/"+strconv.FormatInt(r7.Id, 10)+"/vote", player8, "")
	if code != 200 || !strings.Contains(body, `"votes":2`) {
		t.Fatalf("vote2 = %d body=%s, want votes=2", code, body)
	}

	// 越权：不存在 404、不属该游戏 404
	if code, _ = doAuthed(t, "POST", base+"/games/"+gameID+"/reviews/99999/vote", player7, ""); code != 404 {
		t.Fatalf("vote missing = %d, want 404", code)
	}
	if code, _ = doAuthed(t, "POST", base+"/games/other-game/reviews/"+strconv.FormatInt(r7.Id, 10)+"/vote", player7, ""); code != 404 {
		t.Fatalf("vote wrong game = %d, want 404", code)
	}

	// 列表：默认新在前（r8 先），票数回显
	code, body = doAuthed(t, "GET", base+"/games/"+gameID+"/reviews", "", "")
	if code != 200 {
		t.Fatalf("list = %d", code)
	}
	if !strings.Contains(body, `"votes":2`) || !strings.Contains(body, `"votes":0`) {
		t.Fatalf("list votes missing: %s", body)
	}
	// sort=votes：r7(2) 先于 r8(0)
	code, body = doAuthed(t, "GET", base+"/games/"+gameID+"/reviews?sort=votes", "", "")
	if code != 200 {
		t.Fatalf("list sort=votes = %d", code)
	}
	var list struct {
		Items []struct {
			Id    int64 `json:"id"`
			Votes int64 `json:"votes"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil || len(list.Items) != 2 {
		t.Fatalf("decode list: %v body=%s", err, body)
	}
	if list.Items[0].Id != r7.Id || list.Items[0].Votes != 2 || list.Items[1].Id != r8.Id || list.Items[1].Votes != 0 {
		t.Fatalf("sort=votes items = %+v, want r7(2) then r8(0)", list.Items)
	}

	// 撤：player7 撤一票 → votes=1，voted=false；幂等再撤不变
	code, body = doAuthed(t, "DELETE", base+"/games/"+gameID+"/reviews/"+strconv.FormatInt(r7.Id, 10)+"/vote", player7, "")
	if code != 200 || !strings.Contains(body, `"votes":1`) || !strings.Contains(body, `"voted":false`) {
		t.Fatalf("unvote = %d body=%s", code, body)
	}
	code, body = doAuthed(t, "DELETE", base+"/games/"+gameID+"/reviews/"+strconv.FormatInt(r7.Id, 10)+"/vote", player7, "")
	if code != 200 || !strings.Contains(body, `"votes":1`) {
		t.Fatalf("unvote replay = %d body=%s, want votes=1", code, body)
	}
}
