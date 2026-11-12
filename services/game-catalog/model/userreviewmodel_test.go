package model

// GP-401 评价域模型单测：评分覆盖/聚合、长评唯一约束/列表 JOIN 评分、
// 举报幂等与未决处置、收藏幂等与人群查询。

import (
	"errors"
	"testing"
)

func TestUserRating_UpsertAndSummary(t *testing.T) {
	m := newTestAdminModel(t)

	if rating, err := m.GetUserRating("g-1", 7); err != nil || rating != 0 {
		t.Fatalf("unrated = %d/%v, want 0/nil", rating, err)
	}

	if err := m.UpsertUserRating("g-1", 7, 4); err != nil {
		t.Fatalf("rate: %v", err)
	}
	if err := m.UpsertUserRating("g-1", 8, 2); err != nil {
		t.Fatalf("rate: %v", err)
	}
	// 覆盖式：同一用户重复评分覆盖旧值
	if err := m.UpsertUserRating("g-1", 7, 5); err != nil {
		t.Fatalf("re-rate: %v", err)
	}
	if rating, err := m.GetUserRating("g-1", 7); err != nil || rating != 5 {
		t.Fatalf("rated = %d/%v, want 5", rating, err)
	}

	s, err := m.GetRatingSummary("g-1")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if s.Count != 2 || s.Average < 3.49 || s.Average > 3.51 {
		t.Fatalf("summary = %+v, want count=2 average=3.5", s)
	}
	// 空游戏聚合
	empty, err := m.GetRatingSummary("g-none")
	if err != nil || empty.Count != 0 || empty.Average != 0 {
		t.Fatalf("empty summary = %+v/%v", empty, err)
	}
}

func TestUserReview_CreateUpdateListDelete(t *testing.T) {
	m := newTestAdminModel(t)
	_ = m.UpsertUserRating("g-1", 7, 5)

	r, err := m.CreateUserReview(&UserReview{GameId: "g-1", UserId: 7, Content: "画面和手感都在线，值得入手"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if r.Id <= 0 {
		t.Fatalf("id missing: %+v", r)
	}
	// 一游戏一评：重复插入报哨兵
	if _, err := m.CreateUserReview(&UserReview{GameId: "g-1", UserId: 7, Content: "再来一评应当被唯一约束拒绝"}); !errors.Is(err, ErrUserReviewExists) {
		t.Fatalf("duplicate = %v, want ErrUserReviewExists", err)
	}
	// 第二位玩家的评（列表 JOIN 作者评分）
	if _, err := m.CreateUserReview(&UserReview{GameId: "g-1", UserId: 8, Content: "后期内容略肝，前期体验不错"}); err != nil {
		t.Fatalf("create2: %v", err)
	}

	items, total, err := m.ListUserReviewsByGame("g-1", 20, 0)
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("list = %d/%d/%v", total, len(items), err)
	}
	// 新在前
	if items[0].UserId != 8 || items[1].UserId != 7 {
		t.Fatalf("order wrong: %+v", items)
	}
	// JOIN 评分：user 7 有 5 星，user 8 未评分 0
	if items[1].Rating != 5 || items[0].Rating != 0 {
		t.Fatalf("joined ratings = %d/%d, want 0/5", items[0].Rating, items[1].Rating)
	}

	// 改评
	updated, err := m.UpdateUserReview(r.Id, "改评：通关后补充，剧情后半段更精彩")
	if err != nil || updated.Content != "改评：通关后补充，剧情后半段更精彩" {
		t.Fatalf("update = %+v/%v", updated, err)
	}
	// 开发者回复（覆盖式）
	replied, err := m.UpsertDevReply(r.Id, 42, "感谢反馈，剧情优化在下一版本")
	if err != nil || replied.DevReply == "" || replied.DevReplyBy != 42 {
		t.Fatalf("dev reply = %+v/%v", replied, err)
	}
	replied2, err := m.UpsertDevReply(r.Id, 42, "补充：修复补丁已发布")
	if err != nil || replied2.DevReply != "补充：修复补丁已发布" {
		t.Fatalf("dev reply overwrite = %+v/%v", replied2, err)
	}

	// 不存在
	if _, err := m.GetUserReview(99999); !errors.Is(err, ErrUserReviewNotFound) {
		t.Fatalf("missing = %v, want ErrUserReviewNotFound", err)
	}
	// 删除后列表减一
	if err := m.DeleteUserReview(r.Id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, total, _ = m.ListUserReviewsByGame("g-1", 20, 0)
	if total != 1 {
		t.Fatalf("total after delete = %d, want 1", total)
	}
}

func TestReviewReport_DedupAndDecide(t *testing.T) {
	m := newTestAdminModel(t)
	r, _ := m.CreateUserReview(&UserReview{GameId: "g-1", UserId: 7, Content: "被举报目标评价"})

	rep, created, err := m.CreateReviewReport(&ReviewReport{
		ReviewId: r.Id, GameId: "g-1", ReporterId: 8, Reason: "涉嫌抄袭",
	})
	if err != nil || !created || rep.Status != ReviewReportPending {
		t.Fatalf("report = %+v created=%v err=%v", rep, created, err)
	}
	// 幂等：同举报人同评价返回既有
	rep2, created2, err := m.CreateReviewReport(&ReviewReport{
		ReviewId: r.Id, GameId: "g-1", ReporterId: 8, Reason: "重复举报",
	})
	if err != nil || created2 || rep2.Id != rep.Id || rep2.Reason != "涉嫌抄袭" {
		t.Fatalf("report replay = %+v created=%v err=%v", rep2, created2, err)
	}
	// 不同举报人各自一条
	if _, created3, err := m.CreateReviewReport(&ReviewReport{ReviewId: r.Id, GameId: "g-1", ReporterId: 9, Reason: "低质"}); err != nil || !created3 {
		t.Fatalf("second reporter = %v/%v", created3, err)
	}

	pending, total, err := m.ListReviewReports(ReviewReportPending, 20, 0)
	if err != nil || total != 2 || len(pending) != 2 {
		t.Fatalf("pending list = %d/%d/%v", total, len(pending), err)
	}

	// 未决才可处置（TOCTOU 条件更新）
	ok, err := m.DecideReviewReportIfPending(rep.Id, 1, ReviewReportDismissed)
	if err != nil || !ok {
		t.Fatalf("decide = %v/%v", ok, err)
	}
	// 二次处置不命中
	if ok, _ := m.DecideReviewReportIfPending(rep.Id, 1, ReviewReportResolved); ok {
		t.Fatal("re-decide must not hit")
	}
	// remove 路径：处置 + 删评价 + 同评未决结案
	if ok, err := m.DecideReviewReportIfPending(pending[0].Id, 1, ReviewReportResolved); err != nil || !ok {
		t.Fatalf("decide2 = %v/%v", ok, err)
	}
	if err := m.ResolvePendingReportsForReview(r.Id, 1); err != nil {
		t.Fatalf("resolve siblings: %v", err)
	}
	_, total, _ = m.ListReviewReports(ReviewReportPending, 20, 0)
	if total != 0 {
		t.Fatalf("pending after resolve = %d, want 0", total)
	}
	if err := m.DeleteUserReview(r.Id); err != nil {
		t.Fatalf("delete review: %v", err)
	}
}

func TestGameFavorites_IdempotentAndAudience(t *testing.T) {
	m := newTestAdminModel(t)

	if _, created, err := m.AddFavorite("g-1", 7); err != nil || !created {
		t.Fatalf("add = created %v err %v", created, err)
	}
	if _, created, err := m.AddFavorite("g-1", 7); err != nil || created {
		t.Fatalf("replay = created %v err %v", created, err)
	}
	_, _, _ = m.AddFavorite("g-1", 8)
	_, _, _ = m.AddFavorite("g-2", 7)

	favs, err := m.ListMyFavorites(7, 100)
	if err != nil || len(favs) != 2 {
		t.Fatalf("my favorites = %d/%v", len(favs), err)
	}
	users, err := m.ListFavoriteUsersByGame("g-1", 10000)
	if err != nil || len(users) != 2 || users[0] != 7 || users[1] != 8 {
		t.Fatalf("audience = %v/%v", users, err)
	}
	if err := m.RemoveFavorite("g-1", 7); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := m.RemoveFavorite("g-1", 7); err != nil {
		t.Fatalf("remove replay: %v", err)
	}
	favs, _ = m.ListMyFavorites(7, 100)
	if len(favs) != 1 || favs[0].GameId != "g-2" {
		t.Fatalf("after remove = %+v", favs)
	}
	users, _ = m.ListFavoriteUsersByGame("g-1", 10000)
	if len(users) != 1 || users[0] != 8 {
		t.Fatalf("audience after remove = %v", users)
	}
}
