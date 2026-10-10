package topic

import (
	"context"
	"testing"

	"github.com/tappi/tappi/services/community/internal/types"
)

// --- GetTopic：命中与未命中 ---

func TestGetTopic_ReturnsCreatedTopic(t *testing.T) {
	svcCtx := newTopicSvcCtx(t)
	created, err := NewCreateTopicLogic(authCtx(1), svcCtx).
		CreateTopic(&types.CreateTopicReq{Name: "开荒"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	resp, err := NewGetTopicLogic(context.Background(), svcCtx).
		GetTopic(&types.GetTopicReq{Id: created.Topic.Id})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if resp.Topic.Id != created.Topic.Id || resp.Topic.Name != "开荒" {
		t.Fatalf("topic = %+v, want created one", resp.Topic)
	}
}

func TestGetTopic_NotFound(t *testing.T) {
	_, err := NewGetTopicLogic(context.Background(), newTopicSvcCtx(t)).
		GetTopic(&types.GetTopicReq{Id: 424242})
	if err == nil {
		t.Fatal("expected not-found error for missing topic")
	}
}

// --- FollowTopic / UnfollowTopic：关注流计数增减与幂等 ---

func TestFollowTopic_MissingTopic(t *testing.T) {
	_, err := NewFollowTopicLogic(authCtx(1), newTopicSvcCtx(t)).
		FollowTopic(&types.FollowTopicReq{TopicId: 424242})
	if err == nil {
		t.Fatal("expected error following missing topic")
	}
}

func TestUnfollowTopic_MissingTopic(t *testing.T) {
	_, err := NewUnfollowTopicLogic(authCtx(1), newTopicSvcCtx(t)).
		UnfollowTopic(&types.FollowTopicReq{TopicId: 424242})
	if err == nil {
		t.Fatal("expected error unfollowing missing topic")
	}
}

func TestFollowTopic_FollowAndUnfollowAdjustCounts(t *testing.T) {
	svcCtx := newTopicSvcCtx(t)
	created, err := NewCreateTopicLogic(authCtx(1), svcCtx).
		CreateTopic(&types.CreateTopicReq{Name: "阵容"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := created.Topic.Id

	if _, err := NewFollowTopicLogic(authCtx(7), svcCtx).
		FollowTopic(&types.FollowTopicReq{TopicId: id}); err != nil {
		t.Fatalf("follow: %v", err)
	}
	got, err := NewGetTopicLogic(context.Background(), svcCtx).
		GetTopic(&types.GetTopicReq{Id: id})
	if err != nil {
		t.Fatalf("get after follow: %v", err)
	}
	if got.Topic.FollowerCount != 1 {
		t.Fatalf("follower_count = %d, want 1", got.Topic.FollowerCount)
	}

	// 重复关注幂等：计数不再增加
	if _, err := NewFollowTopicLogic(authCtx(7), svcCtx).
		FollowTopic(&types.FollowTopicReq{TopicId: id}); err != nil {
		t.Fatalf("repeat follow: %v", err)
	}
	got2, _ := NewGetTopicLogic(context.Background(), svcCtx).
		GetTopic(&types.GetTopicReq{Id: id})
	if got2.Topic.FollowerCount != 1 {
		t.Fatalf("follower_count after repeat follow = %d, want 1", got2.Topic.FollowerCount)
	}

	if _, err := NewUnfollowTopicLogic(authCtx(7), svcCtx).
		UnfollowTopic(&types.FollowTopicReq{TopicId: id}); err != nil {
		t.Fatalf("unfollow: %v", err)
	}
	got3, _ := NewGetTopicLogic(context.Background(), svcCtx).
		GetTopic(&types.GetTopicReq{Id: id})
	if got3.Topic.FollowerCount != 0 {
		t.Fatalf("follower_count after unfollow = %d, want 0", got3.Topic.FollowerCount)
	}
}
