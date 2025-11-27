package logic

import (
	"github.com/tappi/tappi/services/content-service/internal/types"
	"github.com/tappi/tappi/services/content-service/model"
)

// Guide转换函数
func modelGuideToType(m *model.Guide) types.Guide {
	if m == nil {
		return types.Guide{}
	}
	return types.Guide{
		Id:          m.Id,
		GameId:      m.GameId,
		GameTitle:   m.GameTitle,
		Title:       m.Title,
		Content:     m.Content,
		Summary:     m.Summary,
		CoverImage:  m.CoverImage,
		AuthorId:    m.AuthorId,
		AuthorName:  m.AuthorName,
		Tags:        m.Tags,
		Views:       m.Views,
		Likes:       m.Likes,
		IsPublished: m.IsPublished,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func modelGuidesToTypes(models []*model.Guide) []types.Guide {
	result := make([]types.Guide, 0, len(models))
	for _, m := range models {
		result = append(result, modelGuideToType(m))
	}
	return result
}

// Comment转换函数
func modelCommentToType(m *model.Comment) types.Comment {
	if m == nil {
		return types.Comment{}
	}
	return types.Comment{
		Id:         m.Id,
		TargetType: m.TargetType,
		TargetId:   m.TargetId,
		UserId:     m.UserId,
		UserName:   m.UserName,
		Content:    m.Content,
		ParentId:   m.ParentId,
		ReplyToId:  m.ReplyToId,
		Likes:      m.Likes,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}
}

func modelCommentsToTypes(models []*model.Comment) []types.Comment {
	result := make([]types.Comment, 0, len(models))
	for _, m := range models {
		result = append(result, modelCommentToType(m))
	}
	return result
}
