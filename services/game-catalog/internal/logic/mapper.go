package logic

import (
	"strings"

	"github.com/tappi/tappi/services/game-catalog/internal/store"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"
)

func toTypesGame(game *model.Game) types.Game {
	if game == nil {
		return types.Game{}
	}
	return types.Game{
		Id:               game.Id,
		Title:            game.Title,
		Description:      game.Description,
		Genres:           append([]string{}, game.Genres...),
		Platforms:        append([]string{}, game.Platforms...),
		ReleaseDate:      game.ReleaseDate,
		Developer:        game.Developer,
		Publisher:        game.Publisher,
		Tags:             append([]string{}, game.Tags...),
		Score:            game.Score,
		TrendingScore:    game.TrendingScore,
		CoverImage:       game.CoverImage,
		Screenshots:      append([]string{}, game.Screenshots...),
		VideoUrl:         game.VideoUrl,
		Changelog:        game.Changelog,
		DeveloperWebsite: game.DeveloperWebsite,
		StoreLink:        game.StoreLink,
		DownloadUrl:      game.DownloadUrl,
		PlayUrl:          game.PlayUrl,
		Status:           game.Status,
		ReviewReason:     game.ReviewReason,
	}
}

func toTypesGames(list []*model.Game) []types.Game {
	result := make([]types.Game, 0, len(list))
	for _, g := range list {
		result = append(result, toTypesGame(g))
	}
	return result
}

func toPartner(p *model.Partner) *types.Partner {
	if p == nil {
		return &types.Partner{}
	}
	return &types.Partner{
		Id: p.Id, Name: p.Name, ContactEmail: p.ContactEmail, Status: p.Status, CreatedAt: p.CreatedAt,
	}
}

func toOwnership(o *model.GameOwnership) *types.GameOwnership {
	if o == nil {
		return &types.GameOwnership{}
	}
	return &types.GameOwnership{
		GameId: o.GameId, PartnerId: o.PartnerId, GrantedBy: o.GrantedBy,
		GrantedAt: o.GrantedAt, Status: o.Status,
	}
}

func toReview(r *model.GameReview) *types.GameReview {
	if r == nil {
		return &types.GameReview{}
	}
	return &types.GameReview{
		Id: r.Id, GameId: r.GameId, VersionId: r.VersionId, SubmittedBy: r.SubmittedBy,
		SubmittedAt: r.SubmittedAt, Status: r.Status, ReviewerId: r.ReviewerId,
		ReviewReason: r.ReviewReason, ReviewedAt: r.ReviewedAt,
	}
}

func toVersion(v *model.GameVersion) *types.GameVersion {
	if v == nil {
		return &types.GameVersion{}
	}
	return &types.GameVersion{
		Id: v.Id, GameId: v.GameId, Version: v.Version, Changelog: v.Changelog,
		PackageName: v.PackageName, VersionCode: v.VersionCode, SignFingerprint: v.SignFingerprint,
		SizeBytes: v.SizeBytes, Hash: v.Hash, StorageUrl: v.StorageUrl,
		Status: v.Status, ReviewReason: v.ReviewReason, CreatedAt: v.CreatedAt,
		// GP-105：包体托管在平台（package:// 协议）才可受控下载
		Downloadable: strings.HasPrefix(v.StorageUrl, store.PackageURLScheme),
	}
}

func toAudit(a *model.AuditLog) *types.AuditLog {
	if a == nil {
		return &types.AuditLog{}
	}
	return &types.AuditLog{
		Id: a.Id, ActorId: a.ActorId, ActorRole: a.ActorRole, Action: a.Action,
		ResourceType: a.ResourceType, ResourceId: a.ResourceId,
		BeforeJson: a.BeforeJson, AfterJson: a.AfterJson, Ip: a.Ip, CreatedAt: a.CreatedAt,
	}
}

func toTypesServer(s *model.GameServer) *types.GameServer {
	if s == nil {
		return &types.GameServer{}
	}
	return &types.GameServer{
		Id: s.Id, GameId: s.GameId, Name: s.Name, Region: s.Region,
		Status: s.Status, Sort: s.Sort, PlannedOpenAt: s.PlannedOpenAt,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}

// toTypesUserReview 玩家长评视图（GP-401 F04）。
func toTypesUserReview(r *model.UserReview) *types.UserReviewView {
	if r == nil {
		return &types.UserReviewView{}
	}
	return &types.UserReviewView{
		Id: r.Id, GameId: r.GameId, UserId: r.UserId, Rating: r.Rating,
		Content: r.Content, DevReply: r.DevReply, DevReplyBy: r.DevReplyBy,
		DevReplyAt: r.DevReplyAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// toTypesReviewReport 举报记录视图（GP-401 F04 管理面）。
func toTypesReviewReport(r *model.ReviewReport) *types.ReviewReportView {
	if r == nil {
		return &types.ReviewReportView{}
	}
	return &types.ReviewReportView{
		Id: r.Id, ReviewId: r.ReviewId, GameId: r.GameId, ReporterId: r.ReporterId,
		Reason: r.Reason, Status: r.Status, HandledBy: r.HandledBy,
		CreatedAt: r.CreatedAt, HandledAt: r.HandledAt,
	}
}

// toTypesPromoChannel 渠道登记视图（GP-403 F22）：渠道为单层，无上级渠道字段。
func toTypesPromoChannel(c *model.PromoChannel) *types.PromoChannelView {
	if c == nil {
		return &types.PromoChannelView{}
	}
	return &types.PromoChannelView{
		Id: c.Id, Code: c.Code, Name: c.Name,
		AttributionWindowHours: c.AttributionWindowHours,
		Status:                 c.Status, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

func toTypesPromoChannels(list []*model.PromoChannel) []types.PromoChannelView {
	out := make([]types.PromoChannelView, 0, len(list))
	for _, c := range list {
		out = append(out, *toTypesPromoChannel(c))
	}
	return out
}

// toTypesChannelStats 渠道包/渠道效果计数视图（GP-403 渠道包追踪）。
func toTypesChannelStats(code string, s *model.ChannelStats) types.PromoChannelStatsResponse {
	resp := types.PromoChannelStatsResponse{Channel: code}
	if s == nil {
		return resp
	}
	resp.Clicks, resp.Downloads, resp.AttributedUsers = s.Clicks, s.Downloads, s.Attributes
	return resp
}
