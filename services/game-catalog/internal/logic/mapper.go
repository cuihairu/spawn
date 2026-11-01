package logic

import (
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
		Status: v.Status, ReviewReason: v.ReviewReason, CreatedAt: v.CreatedAt,
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
