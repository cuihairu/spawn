package logic

import (
	"context"
	"errors"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"
)

// GP-401 评价域 logic 层错误分支覆盖：failingAdminStore 内嵌真实仓储，仅按
// 字段注入指定方法的错误/返回，触达各「查询/写入失败 → 500」与哨兵 → 404/409
// 分支；未覆写的方法委托真实仓储，种子数据照常可用。
type failingAdminStore struct {
	model.AdminStore
	upsertUserRatingErr   error
	getRatingSummaryErr   error
	createUserReviewErr   error
	getUserReviewErrs     []error // 按调用次序：nil=委托真实仓储
	getUserCalls          int
	updateUserReviewErr   error
	listUserReviewsErr    error
	voteReviewErr         error
	unvoteReviewErr       error
	createReviewReportErr error
	upsertDevReplyErr     error
	getRoleWithPartnerErr error
	getOwnershipErr       error
	writeAuditErr         error
	listReviewReportsErr  error
	getReviewReportErrs   []error
	getReportCalls        int
	decideErr             error
	decideOK              *bool
	deleteUserReviewErr   error
	restoreUserReviewErr  error
	restoreOK             *bool
}

var _ model.AdminStore = (*failingAdminStore)(nil)

func (f *failingAdminStore) UpsertUserRating(gameId string, userId int64, rating int) error {
	if f.upsertUserRatingErr != nil {
		return f.upsertUserRatingErr
	}
	return f.AdminStore.UpsertUserRating(gameId, userId, rating)
}

func (f *failingAdminStore) GetRatingSummary(gameId string) (*model.RatingSummary, error) {
	if f.getRatingSummaryErr != nil {
		return nil, f.getRatingSummaryErr
	}
	return f.AdminStore.GetRatingSummary(gameId)
}

func (f *failingAdminStore) CreateUserReview(r *model.UserReview) (*model.UserReview, error) {
	if f.createUserReviewErr != nil {
		return nil, f.createUserReviewErr
	}
	return f.AdminStore.CreateUserReview(r)
}

func (f *failingAdminStore) GetUserReview(id int64) (*model.UserReview, error) {
	if f.getUserReviewErrs != nil {
		f.getUserCalls++
		if i := f.getUserCalls - 1; i >= 0 && i < len(f.getUserReviewErrs) {
			if err := f.getUserReviewErrs[i]; err != nil {
				return nil, err
			}
		}
	}
	return f.AdminStore.GetUserReview(id)
}

func (f *failingAdminStore) UpdateUserReview(id int64, content string) (*model.UserReview, error) {
	if f.updateUserReviewErr != nil {
		return nil, f.updateUserReviewErr
	}
	return f.AdminStore.UpdateUserReview(id, content)
}

func (f *failingAdminStore) ListUserReviewsByGame(gameId string, limit, offset int, byVotes bool) ([]*model.UserReview, int, error) {
	if f.listUserReviewsErr != nil {
		return nil, 0, f.listUserReviewsErr
	}
	return f.AdminStore.ListUserReviewsByGame(gameId, limit, offset, byVotes)
}

func (f *failingAdminStore) VoteReview(gameId string, reviewId, userId int64) (int64, error) {
	if f.voteReviewErr != nil {
		return 0, f.voteReviewErr
	}
	return f.AdminStore.VoteReview(gameId, reviewId, userId)
}

func (f *failingAdminStore) UnvoteReview(gameId string, reviewId, userId int64) (int64, error) {
	if f.unvoteReviewErr != nil {
		return 0, f.unvoteReviewErr
	}
	return f.AdminStore.UnvoteReview(gameId, reviewId, userId)
}

func (f *failingAdminStore) CreateReviewReport(r *model.ReviewReport) (*model.ReviewReport, bool, error) {
	if f.createReviewReportErr != nil {
		return nil, false, f.createReviewReportErr
	}
	return f.AdminStore.CreateReviewReport(r)
}

func (f *failingAdminStore) UpsertDevReply(id, by int64, content string) (*model.UserReview, error) {
	if f.upsertDevReplyErr != nil {
		return nil, f.upsertDevReplyErr
	}
	return f.AdminStore.UpsertDevReply(id, by, content)
}

func (f *failingAdminStore) GetRoleWithPartner(userId int64) (string, int64, error) {
	if f.getRoleWithPartnerErr != nil {
		return "", 0, f.getRoleWithPartnerErr
	}
	return f.AdminStore.GetRoleWithPartner(userId)
}

func (f *failingAdminStore) GetOwnership(gameId string) (*model.GameOwnership, error) {
	if f.getOwnershipErr != nil {
		return nil, f.getOwnershipErr
	}
	return f.AdminStore.GetOwnership(gameId)
}

func (f *failingAdminStore) WriteAudit(l *model.AuditLog) error {
	if f.writeAuditErr != nil {
		return f.writeAuditErr
	}
	return f.AdminStore.WriteAudit(l)
}

func (f *failingAdminStore) ListReviewReports(status string, limit, offset int) ([]*model.ReviewReport, int, error) {
	if f.listReviewReportsErr != nil {
		return nil, 0, f.listReviewReportsErr
	}
	return f.AdminStore.ListReviewReports(status, limit, offset)
}

func (f *failingAdminStore) GetReviewReport(id int64) (*model.ReviewReport, error) {
	if f.getReviewReportErrs != nil {
		f.getReportCalls++
		if i := f.getReportCalls - 1; i >= 0 && i < len(f.getReviewReportErrs) {
			if err := f.getReviewReportErrs[i]; err != nil {
				return nil, err
			}
		}
	}
	return f.AdminStore.GetReviewReport(id)
}

func (f *failingAdminStore) DecideReviewReportIfPending(id, handledBy int64, status string) (bool, error) {
	if f.decideErr != nil {
		return false, f.decideErr
	}
	if f.decideOK != nil {
		return *f.decideOK, nil
	}
	return f.AdminStore.DecideReviewReportIfPending(id, handledBy, status)
}

func (f *failingAdminStore) DeleteUserReview(id int64) error {
	if f.deleteUserReviewErr != nil {
		return f.deleteUserReviewErr
	}
	return f.AdminStore.DeleteUserReview(id)
}

func (f *failingAdminStore) RestoreUserReview(id int64) (bool, error) {
	if f.restoreUserReviewErr != nil {
		return false, f.restoreUserReviewErr
	}
	if f.restoreOK != nil {
		return *f.restoreOK, nil
	}
	return f.AdminStore.RestoreUserReview(id)
}

// newReviewLogicCtx 评价域 logic 测试上下文：真实 SQLite 仓储 + 一个已上架
// 游戏（公开读口径）+ 一条玩家 7 的长评 + 一条玩家 9 的未决举报 + 合作方 5
// 账号 8 的绑定（开发者回复/归属口径）；返回种子长评与举报 id。
func newReviewLogicCtx(t *testing.T) (*svc.ServiceContext, int64, int64) {
	t.Helper()
	svcCtx := newLogicSvcCtx(t)
	if _, err := svcCtx.GameRepository.SetStatus("g-rpg-1", model.GameStatusApproved, ""); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	admin := svcCtx.AdminRepository
	if err := admin.SetRole(8, model.RolePartner, 5); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	if _, err := admin.AssignOwnership("g-rpg-1", 5, 1); err != nil {
		t.Fatalf("AssignOwnership: %v", err)
	}
	r, err := admin.CreateUserReview(&model.UserReview{
		GameId: "g-rpg-1", UserId: 7, Content: "这个游戏的剧情很有代入感",
	})
	if err != nil {
		t.Fatalf("CreateUserReview: %v", err)
	}
	rep, _, err := admin.CreateReviewReport(&model.ReviewReport{
		ReviewId: r.Id, GameId: "g-rpg-1", ReporterId: 9, Reason: "含有不实信息",
	})
	if err != nil {
		t.Fatalf("CreateReviewReport: %v", err)
	}
	return svcCtx, r.Id, rep.Id
}

// reviewCtx 带平台账号的 context（logic.userID() 取 ctx 值）。
func reviewCtx(userID int64) context.Context {
	return context.WithValue(context.Background(), "user_id", userID)
}

// 评分写入失败 → 500。
func TestReviewLogic_RateUpsertFailed500(t *testing.T) {
	svcCtx, _, _ := newReviewLogicCtx(t)
	svcCtx.AdminRepository = &failingAdminStore{
		AdminStore:          svcCtx.AdminRepository,
		upsertUserRatingErr: errors.New("repository on fire"),
	}
	l := NewUserReviewLogic(reviewCtx(7), svcCtx)
	_, err := l.Rate(&types.RateGameRequest{Id: "g-rpg-1", Rating: 5})
	requireAPIStatus(t, err, 500)
}

// 评分写入成功但聚合回读失败 → 500。
func TestReviewLogic_RateSummaryFailed500(t *testing.T) {
	svcCtx, _, _ := newReviewLogicCtx(t)
	svcCtx.AdminRepository = &failingAdminStore{
		AdminStore:          svcCtx.AdminRepository,
		getRatingSummaryErr: errors.New("summary down"),
	}
	l := NewUserReviewLogic(reviewCtx(7), svcCtx)
	_, err := l.Rate(&types.RateGameRequest{Id: "g-rpg-1", Rating: 4})
	requireAPIStatus(t, err, 500)
}

// 长评落库失败 → 500；唯一键冲突 → 409。
func TestReviewLogic_CreateReviewErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
	}{
		{"generic repository error", errors.New("review down"), 500},
		{"unique key already reviewed", model.ErrUserReviewExists, 409},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svcCtx, _, _ := newReviewLogicCtx(t)
			svcCtx.AdminRepository = &failingAdminStore{
				AdminStore:          svcCtx.AdminRepository,
				createUserReviewErr: c.err,
			}
			l := NewUserReviewLogic(reviewCtx(7), svcCtx)
			_, err := l.CreateReview(&types.CreateGameReviewRequest{
				Id: "g-rpg-1", Content: "这个游戏的剧情很有代入感",
			})
			requireAPIStatus(t, err, c.code)
		})
	}
}

// 改评：评价读取失败 → 500；写入失败 → 500。
func TestReviewLogic_UpdateReviewErrors(t *testing.T) {
	t.Run("get failed 500", func(t *testing.T) {
		svcCtx, _, _ := newReviewLogicCtx(t)
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore:        svcCtx.AdminRepository,
			getUserReviewErrs: []error{errors.New("query down")},
		}
		l := NewUserReviewLogic(reviewCtx(7), svcCtx)
		_, err := l.UpdateReview(&types.UpdateGameReviewRequest{
			Id: "g-rpg-1", ReviewId: 1, Content: "改评后的正文仍然足够长",
		})
		requireAPIStatus(t, err, 500)
	})
	t.Run("write failed 500", func(t *testing.T) {
		svcCtx, reviewID, _ := newReviewLogicCtx(t)
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore:          svcCtx.AdminRepository,
			updateUserReviewErr: errors.New("update down"),
		}
		l := NewUserReviewLogic(reviewCtx(7), svcCtx)
		_, err := l.UpdateReview(&types.UpdateGameReviewRequest{
			Id: "g-rpg-1", ReviewId: reviewID, Content: "改评后的正文仍然足够长",
		})
		requireAPIStatus(t, err, 500)
	})
}

// 公开列表查询失败 → 500。
func TestReviewLogic_ListReviewsFailed500(t *testing.T) {
	svcCtx, _, _ := newReviewLogicCtx(t)
	svcCtx.AdminRepository = &failingAdminStore{
		AdminStore:         svcCtx.AdminRepository,
		listUserReviewsErr: errors.New("list down"),
	}
	l := NewUserReviewLogic(reviewCtx(7), svcCtx)
	_, err := l.ListReviews(&types.GameReviewListRequest{Id: "g-rpg-1"})
	requireAPIStatus(t, err, 500)
}

// 有用票：评价不存在（哨兵）→ 404；其余错误 → 500。
func TestReviewLogic_VoteReviewErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
	}{
		{"not found", model.ErrUserReviewNotFound, 404},
		{"generic", errors.New("vote down"), 500},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svcCtx, _, _ := newReviewLogicCtx(t)
			svcCtx.AdminRepository = &failingAdminStore{
				AdminStore:    svcCtx.AdminRepository,
				voteReviewErr: c.err,
			}
			l := NewUserReviewLogic(reviewCtx(7), svcCtx)
			_, err := l.VoteReview(&types.VoteGameReviewRequest{Id: "g-rpg-1", ReviewId: 1})
			requireAPIStatus(t, err, c.code)
		})
	}
}

// 取消投票失败 → 500。
func TestReviewLogic_UnvoteReviewFailed500(t *testing.T) {
	svcCtx, _, _ := newReviewLogicCtx(t)
	svcCtx.AdminRepository = &failingAdminStore{
		AdminStore:      svcCtx.AdminRepository,
		unvoteReviewErr: errors.New("unvote down"),
	}
	l := NewUserReviewLogic(reviewCtx(7), svcCtx)
	_, err := l.UnvoteReview(&types.VoteGameReviewRequest{Id: "g-rpg-1", ReviewId: 1})
	requireAPIStatus(t, err, 500)
}

// 举报：评价读取失败按不存在 404；举报落库失败 500。
func TestReviewLogic_ReportReviewErrors(t *testing.T) {
	t.Run("review missing 404", func(t *testing.T) {
		svcCtx, _, _ := newReviewLogicCtx(t)
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore:        svcCtx.AdminRepository,
			getUserReviewErrs: []error{errors.New("query down")},
		}
		l := NewUserReviewLogic(reviewCtx(7), svcCtx)
		_, err := l.ReportReview(&types.ReportGameReviewRequest{
			Id: "g-rpg-1", ReviewId: 1, Reason: "内容不实",
		})
		requireAPIStatus(t, err, 404)
	})
	t.Run("create report failed 500", func(t *testing.T) {
		svcCtx, reviewID, _ := newReviewLogicCtx(t)
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore:            svcCtx.AdminRepository,
			createReviewReportErr: errors.New("report down"),
		}
		l := NewUserReviewLogic(reviewCtx(7), svcCtx)
		_, err := l.ReportReview(&types.ReportGameReviewRequest{
			Id: "g-rpg-1", ReviewId: reviewID, Reason: "内容不实",
		})
		requireAPIStatus(t, err, 500)
	})
}

// 开发者回复：账号未绑定合作方 403；归属缺失 404；写入失败 500；审计写入
// 失败仅记日志（处置结果不受影响）。
func TestReviewLogic_ReplyReviewErrors(t *testing.T) {
	t.Run("partner unbound 403", func(t *testing.T) {
		svcCtx, _, _ := newReviewLogicCtx(t)
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore:            svcCtx.AdminRepository,
			getRoleWithPartnerErr: errors.New("role down"),
		}
		l := NewUserReviewLogic(reviewCtx(8), svcCtx)
		_, err := l.ReplyReview(&types.ReplyGameReviewRequest{
			Id: "g-rpg-1", ReviewId: 1, Content: "感谢反馈，已记录",
		})
		requireAPIStatus(t, err, 403)
	})
	t.Run("ownership missing 404", func(t *testing.T) {
		svcCtx, _, _ := newReviewLogicCtx(t)
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore:      svcCtx.AdminRepository,
			getOwnershipErr: model.ErrOwnershipMissing,
		}
		l := NewUserReviewLogic(reviewCtx(8), svcCtx)
		_, err := l.ReplyReview(&types.ReplyGameReviewRequest{
			Id: "g-rpg-1", ReviewId: 1, Content: "感谢反馈，已记录",
		})
		requireAPIStatus(t, err, 404)
	})
	t.Run("write failed 500", func(t *testing.T) {
		svcCtx, reviewID, _ := newReviewLogicCtx(t)
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore:        svcCtx.AdminRepository,
			upsertDevReplyErr: errors.New("reply down"),
		}
		l := NewUserReviewLogic(reviewCtx(8), svcCtx)
		_, err := l.ReplyReview(&types.ReplyGameReviewRequest{
			Id: "g-rpg-1", ReviewId: reviewID, Content: "感谢反馈，已记录",
		})
		requireAPIStatus(t, err, 500)
	})
	t.Run("audit failed tolerated", func(t *testing.T) {
		svcCtx, reviewID, _ := newReviewLogicCtx(t)
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore:    svcCtx.AdminRepository,
			writeAuditErr: errors.New("audit down"),
		}
		l := NewUserReviewLogic(reviewCtx(8), svcCtx)
		resp, err := l.ReplyReview(&types.ReplyGameReviewRequest{
			Id: "g-rpg-1", ReviewId: reviewID, Content: "感谢反馈，已记录",
		})
		if err != nil || resp == nil {
			t.Fatalf("audit failure should be tolerated: resp=%+v err=%v", resp, err)
		}
	})
}

// 管理面举报队列：列表查询失败 → 500。
func TestReviewLogic_ListReviewReportsFailed500(t *testing.T) {
	svcCtx, _, _ := newReviewLogicCtx(t)
	svcCtx.AdminRepository = &failingAdminStore{
		AdminStore:           svcCtx.AdminRepository,
		listReviewReportsErr: errors.New("queue down"),
	}
	l := NewUserReviewLogic(reviewCtx(1), svcCtx)
	_, err := l.ListReviewReports(&types.ListReviewReportsRequest{})
	requireAPIStatus(t, err, 500)
}

// 处置举报：读取失败（哨兵/通用）→ 404/500；条件更新失败 → 500；已处置 → 409；
// remove 删评失败 → 500；处置后回读失败 → 500。
func TestReviewLogic_HandleReviewReportErrors(t *testing.T) {
	t.Run("not found 404", func(t *testing.T) {
		svcCtx, _, _ := newReviewLogicCtx(t)
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore:          svcCtx.AdminRepository,
			getReviewReportErrs: []error{model.ErrReviewReportNotFound},
		}
		l := NewUserReviewLogic(reviewCtx(1), svcCtx)
		_, err := l.HandleReviewReport(&types.HandleReviewReportRequest{Id: 99, Action: "dismiss"})
		requireAPIStatus(t, err, 404)
	})
	t.Run("query failed 500", func(t *testing.T) {
		svcCtx, _, _ := newReviewLogicCtx(t)
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore:          svcCtx.AdminRepository,
			getReviewReportErrs: []error{errors.New("query down")},
		}
		l := NewUserReviewLogic(reviewCtx(1), svcCtx)
		_, err := l.HandleReviewReport(&types.HandleReviewReportRequest{Id: 99, Action: "dismiss"})
		requireAPIStatus(t, err, 500)
	})
	t.Run("decide failed 500", func(t *testing.T) {
		svcCtx, _, reportID := newReviewLogicCtx(t)
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore: svcCtx.AdminRepository,
			decideErr:  errors.New("decide down"),
		}
		l := NewUserReviewLogic(reviewCtx(1), svcCtx)
		_, err := l.HandleReviewReport(&types.HandleReviewReportRequest{Id: reportID, Action: "dismiss"})
		requireAPIStatus(t, err, 500)
	})
	t.Run("already handled 409", func(t *testing.T) {
		svcCtx, _, reportID := newReviewLogicCtx(t)
		ok := false
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore: svcCtx.AdminRepository,
			decideOK:   &ok,
		}
		l := NewUserReviewLogic(reviewCtx(1), svcCtx)
		_, err := l.HandleReviewReport(&types.HandleReviewReportRequest{Id: reportID, Action: "dismiss"})
		requireAPIStatus(t, err, 409)
	})
	t.Run("remove delete failed 500", func(t *testing.T) {
		svcCtx, _, reportID := newReviewLogicCtx(t)
		ok := true
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore:          svcCtx.AdminRepository,
			decideOK:            &ok,
			deleteUserReviewErr: errors.New("delete down"),
		}
		l := NewUserReviewLogic(reviewCtx(1), svcCtx)
		_, err := l.HandleReviewReport(&types.HandleReviewReportRequest{Id: reportID, Action: "remove"})
		requireAPIStatus(t, err, 500)
	})
	t.Run("reload failed 500", func(t *testing.T) {
		svcCtx, _, reportID := newReviewLogicCtx(t)
		ok := true
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore:          svcCtx.AdminRepository,
			decideOK:            &ok,
			getReviewReportErrs: []error{nil, errors.New("reload down")},
		}
		l := NewUserReviewLogic(reviewCtx(1), svcCtx)
		_, err := l.HandleReviewReport(&types.HandleReviewReportRequest{Id: reportID, Action: "remove"})
		requireAPIStatus(t, err, 500)
	})
}

// 恢复被移除的评价：读取失败（哨兵/通用）→ 404/500；恢复写入失败 → 500；
// 并发已被恢复 → 409；恢复后回读失败 → 500。
func TestReviewLogic_RestoreReviewErrors(t *testing.T) {
	t.Run("not found 404", func(t *testing.T) {
		svcCtx, _, _ := newReviewLogicCtx(t)
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore:        svcCtx.AdminRepository,
			getUserReviewErrs: []error{model.ErrUserReviewNotFound},
		}
		l := NewUserReviewLogic(reviewCtx(1), svcCtx)
		_, err := l.RestoreReview(&types.RestoreReviewRequest{Id: 1})
		requireAPIStatus(t, err, 404)
	})
	t.Run("query failed 500", func(t *testing.T) {
		svcCtx, _, _ := newReviewLogicCtx(t)
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore:        svcCtx.AdminRepository,
			getUserReviewErrs: []error{errors.New("query down")},
		}
		l := NewUserReviewLogic(reviewCtx(1), svcCtx)
		_, err := l.RestoreReview(&types.RestoreReviewRequest{Id: 1})
		requireAPIStatus(t, err, 500)
	})
	t.Run("restore failed 500", func(t *testing.T) {
		svcCtx, reviewID, _ := newReviewLogicCtx(t)
		if err := svcCtx.AdminRepository.DeleteUserReview(reviewID); err != nil {
			t.Fatalf("soft delete seed: %v", err)
		}
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore:           svcCtx.AdminRepository,
			restoreUserReviewErr: errors.New("restore down"),
		}
		l := NewUserReviewLogic(reviewCtx(1), svcCtx)
		_, err := l.RestoreReview(&types.RestoreReviewRequest{Id: reviewID})
		requireAPIStatus(t, err, 500)
	})
	t.Run("concurrently restored 409", func(t *testing.T) {
		svcCtx, reviewID, _ := newReviewLogicCtx(t)
		if err := svcCtx.AdminRepository.DeleteUserReview(reviewID); err != nil {
			t.Fatalf("soft delete seed: %v", err)
		}
		ok := false
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore: svcCtx.AdminRepository,
			restoreOK:  &ok,
		}
		l := NewUserReviewLogic(reviewCtx(1), svcCtx)
		_, err := l.RestoreReview(&types.RestoreReviewRequest{Id: reviewID})
		requireAPIStatus(t, err, 409)
	})
	t.Run("reload failed 500", func(t *testing.T) {
		svcCtx, reviewID, _ := newReviewLogicCtx(t)
		if err := svcCtx.AdminRepository.DeleteUserReview(reviewID); err != nil {
			t.Fatalf("soft delete seed: %v", err)
		}
		ok := true
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore:        svcCtx.AdminRepository,
			restoreOK:         &ok,
			getUserReviewErrs: []error{nil, errors.New("reload down")},
		}
		l := NewUserReviewLogic(reviewCtx(1), svcCtx)
		_, err := l.RestoreReview(&types.RestoreReviewRequest{Id: reviewID})
		requireAPIStatus(t, err, 500)
	})
}
