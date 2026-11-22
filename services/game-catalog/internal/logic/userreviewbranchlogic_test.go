package logic

import (
	"errors"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"
)

// 未登录 401：评分/发评/改评/投票/撤票/举报全部入口一致拦截。
func TestReviewLogic_Unauthorized(t *testing.T) {
	svcCtx, reviewID, _ := newReviewLogicCtx(t)
	l := NewUserReviewLogic(reviewCtx(0), svcCtx)
	cases := []struct {
		name string
		call func() error
	}{
		{"rate", func() error { _, e := l.Rate(&types.RateGameRequest{Id: "g-rpg-1", Rating: 5}); return e }},
		{"create", func() error {
			_, e := l.CreateReview(&types.CreateGameReviewRequest{Id: "g-rpg-1", Content: "这个游戏的剧情真的很有代入感"})
			return e
		}},
		{"update", func() error {
			_, e := l.UpdateReview(&types.UpdateGameReviewRequest{Id: "g-rpg-1", ReviewId: reviewID, Content: "这个游戏的剧情真的很有代入感"})
			return e
		}},
		{"vote", func() error {
			_, e := l.VoteReview(&types.VoteGameReviewRequest{Id: "g-rpg-1", ReviewId: reviewID})
			return e
		}},
		{"unvote", func() error {
			_, e := l.UnvoteReview(&types.VoteGameReviewRequest{Id: "g-rpg-1", ReviewId: reviewID})
			return e
		}},
		{"report", func() error {
			_, e := l.ReportReview(&types.ReportGameReviewRequest{Id: "g-rpg-1", ReviewId: reviewID, Reason: "不实"})
			return e
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			requireAPIStatus(t, c.call(), 401)
		})
	}
}

// 可见性口径：游戏未上架 → 404（visibleGame 未上架分支）。
func TestReviewLogic_VisibleGameNotApproved(t *testing.T) {
	svcCtx, reviewID, _ := newReviewLogicCtx(t)
	if _, err := svcCtx.GameRepository.SetStatus("g-rpg-1", model.GameStatusOffline, ""); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	l := NewUserReviewLogic(reviewCtx(7), svcCtx)
	cases := []struct {
		name string
		call func() error
	}{
		{"rate", func() error { _, e := l.Rate(&types.RateGameRequest{Id: "g-rpg-1", Rating: 5}); return e }},
		{"create", func() error {
			_, e := l.CreateReview(&types.CreateGameReviewRequest{Id: "g-rpg-1", Content: "这个游戏的剧情真的很有代入感"})
			return e
		}},
		{"list", func() error { _, e := l.ListReviews(&types.GameReviewListRequest{Id: "g-rpg-1"}); return e }},
		{"vote", func() error {
			_, e := l.VoteReview(&types.VoteGameReviewRequest{Id: "g-rpg-1", ReviewId: reviewID})
			return e
		}},
		{"unvote", func() error {
			_, e := l.UnvoteReview(&types.VoteGameReviewRequest{Id: "g-rpg-1", ReviewId: reviewID})
			return e
		}},
		{"report", func() error {
			_, e := l.ReportReview(&types.ReportGameReviewRequest{Id: "g-rpg-1", ReviewId: reviewID, Reason: "不实"})
			return e
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			requireAPIStatus(t, c.call(), 404)
		})
	}
}

// 可见性口径：游戏查询失败 → 500（visibleGame 查询失败分支）。
func TestReviewLogic_VisibleGameQueryFailed(t *testing.T) {
	svcCtx, reviewID, _ := newReviewLogicCtx(t)
	svcCtx.GameRepository = &failingGameStore{
		GameStore: svcCtx.GameRepository,
		getErr:    errors.New("query down"),
	}
	l := NewUserReviewLogic(reviewCtx(7), svcCtx)
	if _, err := l.Rate(&types.RateGameRequest{Id: "g-rpg-1", Rating: 5}); err != nil {
		requireAPIStatus(t, err, 500)
	} else {
		t.Fatal("want 500")
	}
	// 再触达 vote/report 的 visibleGame 失败分支
	if _, err := l.VoteReview(&types.VoteGameReviewRequest{Id: "g-rpg-1", ReviewId: reviewID}); err != nil {
		requireAPIStatus(t, err, 500)
	}
	if _, err := l.ReportReview(&types.ReportGameReviewRequest{Id: "g-rpg-1", ReviewId: reviewID, Reason: "不实"}); err != nil {
		requireAPIStatus(t, err, 500)
	}
}

// 正文/理由长度口径：超限 → 400。
func TestReviewLogic_LengthValidation(t *testing.T) {
	svcCtx, reviewID, _ := newReviewLogicCtx(t)
	l := NewUserReviewLogic(reviewCtx(7), svcCtx)
	long := strings.Repeat("长", 4001)
	tooLongReport := strings.Repeat("报", 501)

	if _, err := l.CreateReview(&types.CreateGameReviewRequest{Id: "g-rpg-1", Content: long}); err != nil {
		requireAPIStatus(t, err, 400)
	}
	if _, err := l.UpdateReview(&types.UpdateGameReviewRequest{Id: "g-rpg-1", ReviewId: reviewID, Content: long}); err != nil {
		requireAPIStatus(t, err, 400)
	}
	if _, err := l.ReportReview(&types.ReportGameReviewRequest{Id: "g-rpg-1", ReviewId: reviewID, Reason: tooLongReport}); err != nil {
		requireAPIStatus(t, err, 400)
	}
}

// 开发者回复超长 → 400（长度校验在归属校验之后，需以合作方账号触达）。
func TestReviewLogic_ReplyReviewTooLong(t *testing.T) {
	svcCtx, reviewID, _ := newReviewLogicCtx(t)
	l := NewUserReviewLogic(reviewCtx(8), svcCtx)
	if _, err := l.ReplyReview(&types.ReplyGameReviewRequest{
		Id: "g-rpg-1", ReviewId: reviewID, Content: strings.Repeat("复", 2001),
	}); err != nil {
		requireAPIStatus(t, err, 400)
	}
}

// 改评：评价不存在 → 404。
func TestReviewLogic_UpdateReviewNotFound(t *testing.T) {
	svcCtx, _, _ := newReviewLogicCtx(t)
	svcCtx.AdminRepository = &failingAdminStore{
		AdminStore:        svcCtx.AdminRepository,
		getUserReviewErrs: []error{model.ErrUserReviewNotFound},
	}
	l := NewUserReviewLogic(reviewCtx(7), svcCtx)
	_, err := l.UpdateReview(&types.UpdateGameReviewRequest{Id: "g-rpg-1", ReviewId: 999, Content: "这个游戏的剧情真的很有代入感"})
	requireAPIStatus(t, err, 404)
}

// 撤票：评价不存在（哨兵）→ 404。
func TestReviewLogic_UnvoteReviewNotFound(t *testing.T) {
	svcCtx, _, _ := newReviewLogicCtx(t)
	svcCtx.AdminRepository = &failingAdminStore{
		AdminStore:      svcCtx.AdminRepository,
		unvoteReviewErr: model.ErrUserReviewNotFound,
	}
	l := NewUserReviewLogic(reviewCtx(7), svcCtx)
	_, err := l.UnvoteReview(&types.VoteGameReviewRequest{Id: "g-rpg-1", ReviewId: 999})
	requireAPIStatus(t, err, 404)
}

// 开发者回复：评价不存在/不属于该游戏 → 404。
func TestReviewLogic_ReplyReviewNotFound(t *testing.T) {
	svcCtx, _, _ := newReviewLogicCtx(t)
	svcCtx.AdminRepository = &failingAdminStore{
		AdminStore:        svcCtx.AdminRepository,
		getUserReviewErrs: []error{model.ErrUserReviewNotFound},
	}
	l := NewUserReviewLogic(reviewCtx(8), svcCtx)
	_, err := l.ReplyReview(&types.ReplyGameReviewRequest{Id: "g-rpg-1", ReviewId: 999, Content: "感谢反馈"})
	requireAPIStatus(t, err, 404)
}

// 开发者回复：账号绑定合作方 id<=0 → 403。
func TestReviewLogic_ReplyReviewPartnerZero(t *testing.T) {
	svcCtx, _, _ := newReviewLogicCtx(t)
	svcCtx.AdminRepository = &failingAdminStore{
		AdminStore:      svcCtx.AdminRepository,
		rolePartnerZero: true,
	}
	l := NewUserReviewLogic(reviewCtx(8), svcCtx)
	_, err := l.ReplyReview(&types.ReplyGameReviewRequest{Id: "g-rpg-1", ReviewId: 1, Content: "感谢反馈"})
	requireAPIStatus(t, err, 403)
}

// 开发者回复：归属查询通用失败 → 500（ownGameOf 通用错误分支）。
func TestReviewLogic_ReplyReviewOwnershipQueryFailed(t *testing.T) {
	svcCtx, _, _ := newReviewLogicCtx(t)
	svcCtx.AdminRepository = &failingAdminStore{
		AdminStore:      svcCtx.AdminRepository,
		getOwnershipErr: errors.New("ownership down"),
	}
	l := NewUserReviewLogic(reviewCtx(8), svcCtx)
	_, err := l.ReplyReview(&types.ReplyGameReviewRequest{Id: "g-rpg-1", ReviewId: 1, Content: "感谢反馈"})
	requireAPIStatus(t, err, 500)
}

// 处置（remove）通知面：作者读取/未决举报人/结案全部失败仅记日志不阻断，
// 且通知口未启用时扇出被跳过（notify 各 disabled 分支）。
func TestReviewLogic_HandleReportRemoveLogPaths(t *testing.T) {
	svcCtx, _, reportID := newReviewLogicCtx(t)
	ok := true
	svcCtx.Notify = &fakePublisher{enabled: false}
	svcCtx.AdminRepository = &failingAdminStore{
		AdminStore:               svcCtx.AdminRepository,
		decideOK:                 &ok,
		getUserReviewErrs:        []error{errors.New("author fetch down")},
		listPendingReportersErr:  errors.New("pending fetch down"),
		resolvePendingReportsErr: errors.New("resolve down"),
	}
	l := NewUserReviewLogic(reviewCtx(1), svcCtx)
	resp, err := l.HandleReviewReport(&types.HandleReviewReportRequest{Id: reportID, Action: "remove"})
	if err != nil || resp == nil {
		t.Fatalf("notify failures should not block: resp=%+v err=%v", resp, err)
	}
}

// 处置（dismiss）通知面：通知口未启用时扇出跳过，处置结果不受影响。
func TestReviewLogic_HandleReportDismissNotifyDisabled(t *testing.T) {
	svcCtx, _, reportID := newReviewLogicCtx(t)
	ok := true
	svcCtx.Notify = &fakePublisher{enabled: false}
	svcCtx.AdminRepository = &failingAdminStore{
		AdminStore: svcCtx.AdminRepository,
		decideOK:   &ok,
	}
	l := NewUserReviewLogic(reviewCtx(1), svcCtx)
	resp, err := l.HandleReviewReport(&types.HandleReviewReportRequest{Id: reportID, Action: "dismiss"})
	if err != nil || resp == nil {
		t.Fatalf("dismiss should succeed: resp=%+v err=%v", resp, err)
	}
}

// 恢复被移除评价：成功路径 + 通知口未启用扇出跳过。
func TestReviewLogic_RestoreReviewSuccessNotifyDisabled(t *testing.T) {
	svcCtx, reviewID, _ := newReviewLogicCtx(t)
	if err := svcCtx.AdminRepository.DeleteUserReview(reviewID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	svcCtx.Notify = &fakePublisher{enabled: false}
	l := NewUserReviewLogic(reviewCtx(1), svcCtx)
	resp, err := l.RestoreReview(&types.RestoreReviewRequest{Id: reviewID})
	if err != nil || resp == nil || resp.DeletedAt != "" {
		t.Fatalf("restore should succeed: resp=%+v err=%v", resp, err)
	}
}

// distinctPositive 去重并剔除 <=0 的受众 id（通知容错分支）。
func TestDistinctPositive(t *testing.T) {
	got := distinctPositive([]int64{0, 5, 5, -1, 7, 0})
	want := []int64{5, 7}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}
