package logic

// mapper 补漏分支：各 to* 映射器的 nil 守卫臂（nil 入参 → 空结构，不 panic）。
// 正常入参路径已由既有测试覆盖，此处仅补 nil 臂。

import "testing"

func TestMapper_NilInputs(t *testing.T) {
	if got := toTypesGame(nil); got.Id != "" || got.Title != "" || got.Genres != nil {
		t.Fatalf("toTypesGame(nil) = %+v, want zero Game{} with nil slices", got)
	}
	if got := toOwnership(nil); got == nil {
		t.Fatal("toOwnership(nil) = nil, want &types.GameOwnership{}")
	}
	if got := toPartner(nil); got == nil {
		t.Fatal("toPartner(nil) = nil, want &types.Partner{}")
	}
	if got := toReview(nil); got == nil {
		t.Fatal("toReview(nil) = nil, want &types.GameReview{}")
	}
	if got := toVersion(nil); got == nil {
		t.Fatal("toVersion(nil) = nil, want &types.GameVersion{}")
	}
	if got := toAudit(nil); got == nil {
		t.Fatal("toAudit(nil) = nil, want &types.AuditLog{}")
	}
	if got := toTypesServer(nil); got == nil {
		t.Fatal("toTypesServer(nil) = nil, want &types.GameServer{}")
	}
	if got := toTypesUserReview(nil); got == nil {
		t.Fatal("toTypesUserReview(nil) = nil, want &types.UserReviewView{}")
	}
	if got := toTypesReviewReport(nil); got == nil {
		t.Fatal("toTypesReviewReport(nil) = nil, want &types.ReviewReportView{}")
	}
	if got := toTypesPromoChannel(nil); got == nil {
		t.Fatal("toTypesPromoChannel(nil) = nil, want &types.PromoChannelView{}")
	}
	// toTypesChannelStats 值返回：nil → 仅 channel 字段，计数零值。
	if got := toTypesChannelStats("c1", nil); got.Channel != "c1" || got.Clicks != 0 || got.Downloads != 0 || got.AttributedUsers != 0 {
		t.Fatalf("toTypesChannelStats(nil) = %+v, want channel c1 + zero counts", got)
	}
}
