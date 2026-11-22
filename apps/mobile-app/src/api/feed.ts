import { request, gatewayBase } from './http';
import type { Game } from './games';
import type { Topic } from './community';
import type { Guide } from './guides';

// ========== BFF 聚合端点（:8800 自有 handler） ==========

// GET /home/feed：一次请求拉精选游戏 / 热帖 / 话题 / 攻略四个列表，
// 网关侧已做字段裁剪（正文/图片等大字段不下发，正文仅截断出 summary）。
// degraded 列出因上游故障被降级为空列表的分组，UI 可按组做降级提示。
export interface HomeFeed {
  featured_games: Pick<Game, 'id' | 'title' | 'cover_image' | 'score' | 'genres' | 'platforms'>[];
  hot_posts: {
    id: number;
    topic_id: number;
    author_id: number;
    author_name: string;
    title: string;
    summary: string;
    like_count: number;
    comment_count: number;
    created_at: string;
  }[];
  topics: Pick<Topic, 'id' | 'name' | 'post_count' | 'follower_count' | 'is_official'>[];
  guides: Pick<Guide, 'id' | 'game_id' | 'game_title' | 'title' | 'summary' | 'author_name' | 'likes' | 'views' | 'created_at'>[];
  degraded: string[];
}

// limit 为每组条数（网关缺省 5，上限 20）。匿名可用，无需 token。
export async function fetchHomeFeed(limit = 5): Promise<HomeFeed> {
  return request<HomeFeed>(`${gatewayBase}/home/feed?limit=${limit}`);
}
