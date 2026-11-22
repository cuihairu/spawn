package handler

// DESIGN-001 设计拆解域集成测试：种子（空库启动写入 12 篇 × 5 品类）、品类
// 聚合、列表筛选分页、详情全文、登录发布/仅作者编辑、精选仅管理员、讨论区
// 发言/列表/删自己评论、敏感词与品类白名单校验。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/tappi/tappi/services/community/internal/handler/design"
	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
	"github.com/zeromicro/go-zero/rest/pathvar"
)

// designDo 发请求并回记录器（公开读不带 token；写端点的 handler 先经
// svcCtx.Auth 包装，鉴权与生产路由一致）。
func designDo(t *testing.T, h http.HandlerFunc, method, path string,
	vars map[string]string, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if len(vars) > 0 {
		req = pathvar.WithVars(req, vars)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

// designVars 组 :id（可带 :cid 等附加段）路径变量。
func designVars(id int64, extra map[string]string) map[string]string {
	vars := map[string]string{"id": strconv.FormatInt(id, 10)}
	for k, v := range extra {
		vars[k] = v
	}
	return vars
}

func designWantCode(t *testing.T, rr *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rr.Code != want {
		t.Fatalf("status = %d body=%s, want %d", rr.Code, rr.Body.String(), want)
	}
}

func designInto(t *testing.T, rr *httptest.ResponseRecorder, into any) {
	t.Helper()
	if err := json.Unmarshal(rr.Body.Bytes(), into); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rr.Body.String())
	}
}

func TestRoutes_DesignBreakdowns(t *testing.T) {
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)

	svcCtx, secret := newTestServiceContext(t)
	svcCtx.AdminSet = map[int64]bool{1: true}
	adminToken := signToken(t, secret, 1, "admin")
	playerToken := signToken(t, secret, 2, "planner")
	otherToken := signToken(t, secret, 3, "reader")

	// 1. 种子与品类聚合：空库启动写入 12 篇 × 5 品类（NewServiceContext 种子）
	catsRR := designDo(t, design.ListCategoriesHandler(svcCtx), http.MethodGet,
		"/api/v1/design/categories", nil, "", "")
	designWantCode(t, catsRR, http.StatusOK)
	var cats types.DesignCategoriesResp
	designInto(t, catsRR, &cats)
	got := map[string]int64{}
	for _, c := range cats.Categories {
		got[c.Category] = c.Count
	}
	if len(got) != 5 {
		t.Fatalf("categories = %v, want 5", got)
	}
	for _, c := range []string{"MMORPG", "MOBA", "SLG", "卡牌", "放置"} {
		if got[c] < 2 {
			t.Fatalf("category %s count = %d, want >= 2 seeds", c, got[c])
		}
	}

	// 2. 列表：全量 / 品类筛选 / 分页 / 精选（初始为空）；列表条目不带全文
	list := design.ListBreakdownsHandler(svcCtx)
	var listAll types.DesignListResp
	designInto(t, designDo(t, list, http.MethodGet, "/api/v1/design/breakdowns", nil, "", ""), &listAll)
	if listAll.Total != 12 {
		t.Fatalf("total = %d, want 12 seeds", listAll.Total)
	}
	var listCat types.DesignListResp
	catRR := designDo(t, list, http.MethodGet,
		"/api/v1/design/breakdowns?category=%E5%8D%A1%E7%89%8C", nil, "", "")
	designWantCode(t, catRR, http.StatusOK)
	designInto(t, catRR, &listCat)
	if listCat.Total != 3 {
		t.Fatalf("卡牌 total = %d body=%s, want 3", listCat.Total, catRR.Body.String())
	}
	if len(listCat.Items) == 0 || listCat.Items[0].CoreLoop != "" {
		t.Fatalf("list item leaked full sections: %+v", listCat.Items)
	}
	var listPage types.DesignListResp
	designInto(t, designDo(t, list, http.MethodGet,
		"/api/v1/design/breakdowns?limit=5&offset=10", nil, "", ""), &listPage)
	if len(listPage.Items) != 2 {
		t.Fatalf("paged items = %d, want 2", len(listPage.Items))
	}
	var listFeat types.DesignListResp
	designInto(t, designDo(t, list, http.MethodGet,
		"/api/v1/design/breakdowns?featured=true", nil, "", ""), &listFeat)
	if listFeat.Total != 0 {
		t.Fatalf("featured before ops = %d, want 0", listFeat.Total)
	}

	// 3. 详情：全文可读（分节 + 来源）；不存在 404
	var detail types.DesignDetailResp
	designInto(t, designDo(t, design.GetBreakdownHandler(svcCtx), http.MethodGet,
		"/api/v1/design/breakdowns/1", designVars(1, nil), "", ""), &detail)
	if detail.Breakdown.Id != 1 || detail.Breakdown.CoreLoop == "" || detail.Breakdown.Sources == "" {
		t.Fatalf("detail = %+v, want full sections", detail.Breakdown)
	}
	designWantCode(t, designDo(t, design.GetBreakdownHandler(svcCtx), http.MethodGet,
		"/api/v1/design/breakdowns/999", designVars(999, nil), "", ""), http.StatusNotFound)

	// 4. 发布：登录即可；标题必填；品类白名单拒绝
	create := svcCtx.Auth(design.CreateBreakdownHandler(svcCtx))
	createdRR := designDo(t, create, http.MethodPost, "/api/v1/design/breakdowns", nil, adminToken,
		`{"game_title":"测试拆解","category":"MMORPG","summary":"测试摘要","core_loop":"测试核心循环"}`)
	designWantCode(t, createdRR, http.StatusOK)
	var created types.DesignDetailResp
	designInto(t, createdRR, &created)
	if created.Breakdown.Id == 0 || created.Breakdown.AuthorName != "admin" {
		t.Fatalf("created = %+v", created.Breakdown)
	}
	designWantCode(t, designDo(t, create, http.MethodPost, "/api/v1/design/breakdowns", nil, playerToken,
		`{"game_title":"","category":"MMORPG"}`), http.StatusBadRequest)
	designWantCode(t, designDo(t, create, http.MethodPost, "/api/v1/design/breakdowns", nil, playerToken,
		`{"game_title":"X","category":"射击"}`), http.StatusBadRequest)

	// 5. 编辑：仅作者（非作者/不存在统一 404 不泄露可编辑性）；更新后回读生效
	update := svcCtx.Auth(design.UpdateBreakdownHandler(svcCtx))
	editBody := `{"game_title":"测试拆解改","category":"MOBA","summary":"改后摘要"}`
	designWantCode(t, designDo(t, update, http.MethodPut, "/api/v1/design/breakdowns/900",
		designVars(900, nil), playerToken, editBody), http.StatusNotFound)
	createdId := created.Breakdown.Id
	updatedRR := designDo(t, update, http.MethodPut,
		"/api/v1/design/breakdowns/"+strconv.FormatInt(createdId, 10),
		designVars(createdId, nil), adminToken, editBody)
	designWantCode(t, updatedRR, http.StatusOK)
	var updated types.DesignDetailResp
	designInto(t, updatedRR, &updated)
	if updated.Breakdown.GameTitle != "测试拆解改" || updated.Breakdown.Category != "MOBA" {
		t.Fatalf("updated = %+v", updated.Breakdown)
	}

	// 6. 精选：玩家 403；管理员 200 幂等；精选列表出现该条
	feature := svcCtx.Auth(design.FeatureBreakdownHandler(svcCtx))
	featPath := "/api/v1/design/breakdowns/" + strconv.FormatInt(createdId, 10) + "/feature"
	featVars := designVars(createdId, nil)
	designWantCode(t, designDo(t, feature, http.MethodPost, featPath, featVars, playerToken,
		`{"featured":true}`), http.StatusForbidden)
	designWantCode(t, designDo(t, feature, http.MethodPost, featPath, featVars, adminToken,
		`{"featured":true}`), http.StatusOK)
	designWantCode(t, designDo(t, feature, http.MethodPost, featPath, featVars, adminToken,
		`{"featured":true}`), http.StatusOK)
	var featList types.DesignListResp
	designInto(t, designDo(t, list, http.MethodGet,
		"/api/v1/design/breakdowns?featured=true", nil, "", ""), &featList)
	if featList.Total != 1 || len(featList.Items) != 1 || featList.Items[0].Id != createdId {
		t.Fatalf("featured list = %+v, want the featured item", featList)
	}

	// 7. 讨论区：未登录 401；空内容 400；发言；列表升序；他人删 404；本人删成功
	addComment := svcCtx.Auth(design.CreateDesignCommentHandler(svcCtx))
	commentPath := "/api/v1/design/breakdowns/" + strconv.FormatInt(createdId, 10) + "/comments"
	designWantCode(t, designDo(t, addComment, http.MethodPost, commentPath, nil, "",
		`{"content":"hello"}`), http.StatusUnauthorized)
	designWantCode(t, designDo(t, addComment, http.MethodPost, commentPath, featVars, playerToken,
		`{"content":"   "}`), http.StatusBadRequest)

	var comments types.DesignCommentsResp
	var c1, c2 types.DesignComment
	for _, tc := range []struct {
		token   string
		content string
		into    *types.DesignComment
	}{
		{playerToken, "很有参考价值", &c1},
		{otherToken, "互补视角", &c2},
	} {
		rr := designDo(t, addComment, http.MethodPost, commentPath, featVars, tc.token,
			`{"content":"`+tc.content+`"}`)
		designWantCode(t, rr, http.StatusOK)
		var wrapper struct {
			Comment types.DesignComment `json:"comment"`
		}
		designInto(t, rr, &wrapper)
		*tc.into = wrapper.Comment
	}
	if c1.Id == 0 || c1.BreakdownId != createdId {
		t.Fatalf("comment1 = %+v", c1)
	}
	designInto(t, designDo(t, design.ListDesignCommentsHandler(svcCtx), http.MethodGet,
		commentPath, featVars, "", ""), &comments)
	if len(comments.Comments) != 2 || comments.Comments[0].Content != "很有参考价值" {
		t.Fatalf("comments = %+v", comments.Comments)
	}

	delComment := svcCtx.Auth(design.DeleteDesignCommentHandler(svcCtx))
	designWantCode(t, designDo(t, delComment, http.MethodDelete, commentPath+"/"+
		strconv.FormatInt(c1.Id, 10), designVars(createdId, map[string]string{
		"cid": strconv.FormatInt(c1.Id, 10)}), otherToken, ""), http.StatusNotFound)
	designWantCode(t, designDo(t, delComment, http.MethodDelete, commentPath+"/"+
		strconv.FormatInt(c1.Id, 10), designVars(createdId, map[string]string{
		"cid": strconv.FormatInt(c1.Id, 10)}), playerToken, ""), http.StatusOK)
	designInto(t, designDo(t, design.ListDesignCommentsHandler(svcCtx), http.MethodGet,
		commentPath, featVars, "", ""), &comments)
	if len(comments.Comments) != 1 || comments.Comments[0].Id != c2.Id {
		t.Fatalf("comments after delete = %+v", comments.Comments)
	}

	// 8. 敏感词：词表配置后评论命中 → 400
	svcCtx.BlockedWords = []string{"违禁词"}
	designWantCode(t, designDo(t, addComment, http.MethodPost, commentPath, featVars, playerToken,
		`{"content":"这里有违禁词内容"}`), http.StatusBadRequest)
}
