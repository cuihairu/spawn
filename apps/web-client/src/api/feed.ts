// BFF 聚合端点 GET /home/feed 的裁剪契约：网关侧已按列表展示裁剪字段，
// 正文只留 summary；degraded 列出因上游故障被降级为空列表的分组。
import { gatewayBaseUrl, request } from './http'

export interface FeedGame {
  id: string
  title: string
  cover_image?: string
  score: number
  genres: string[]
  platforms: string[]
}

export interface FeedPost {
  id: number
  topic_id: number
  author_id: number
  author_name?: string
  title: string
  summary: string
  like_count: number
  comment_count: number
  created_at: string
}

export interface FeedTopic {
  id: number
  name: string
  post_count: number
  follower_count: number
  is_official: boolean
}

export interface FeedGuide {
  id: number
  game_id: string
  game_title: string
  title: string
  summary: string
  author_name?: string
  likes: number
  views: number
  created_at: string
}

export interface HomeFeed {
  featured_games: FeedGame[]
  hot_posts: FeedPost[]
  topics: FeedTopic[]
  guides: FeedGuide[]
  degraded: string[]
}

export async function fetchHomeFeed(limit = 8): Promise<HomeFeed> {
  const payload = await request<HomeFeed>(gatewayBaseUrl, `/home/feed?limit=${limit}`)
  return { ...payload, degraded: payload.degraded ?? [] }
}
