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
		Id:            game.Id,
		Title:         game.Title,
		Description:   game.Description,
		Genres:        append([]string{}, game.Genres...),
		Platforms:     append([]string{}, game.Platforms...),
		ReleaseDate:   game.ReleaseDate,
		Developer:     game.Developer,
		Publisher:     game.Publisher,
		Tags:          append([]string{}, game.Tags...),
		Score:         game.Score,
		TrendingScore: game.TrendingScore,
		CoverImage:    game.CoverImage,
	}
}

func toTypesGames(list []*model.Game) []types.Game {
	result := make([]types.Game, 0, len(list))
	for _, g := range list {
		result = append(result, toTypesGame(g))
	}
	return result
}
