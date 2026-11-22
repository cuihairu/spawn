import { useCallback, useEffect, useState } from 'react';
import {
  ActivityIndicator,
  Alert,
  Image,
  Linking,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import Ionicons from '@expo/vector-icons/Ionicons';

import {
  addFavorite,
  cancelReservation,
  createGameReview,
  fetchGameById,
  fetchGameReviews,
  fetchGameServers,
  fetchGuides,
  fetchLatestGameVersion,
  fetchMyFavoriteGameIds,
  fetchMyReservations,
  removeFavorite,
  reportGameReview,
  reserveServer,
  submitGameRating,
  updateGameReview,
  type Game,
  type GameReview,
  type GameServerView,
  type Guide,
  type LatestGameVersion,
  type RatingSummary,
} from '../../api/client';
import { useAuth } from '../../auth/AuthContext';
import { colors } from '../../constants/colors';
import { PackageDownload } from '../../components/PackageDownload';

// 游戏详情（Stack 动态路由 /game/[id]）：GET /games/:id（返回 { game, rating } 包裹）。
// GP-401 评分/长评/举报/收藏与 GP-306 区服预约按登录态启用。
export default function GameDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { token, user } = useAuth();
  const [game, setGame] = useState<Game | null>(null);
  const [guides, setGuides] = useState<Guide[]>([]);
  // GP-105 当前已发布版本元数据（受控下载入口）；无发布版本为 null
  const [pkg, setPkg] = useState<LatestGameVersion | null>(null);
  const insets = useSafeAreaInsets();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // GP-401：玩家评分/收藏/长评（含作者改评与举报）
  const [myRating, setMyRating] = useState(0);
  const [ratingBusy, setRatingBusy] = useState(false);
  const [ratingSummary, setRatingSummary] = useState<RatingSummary | null>(null);
  const [favorited, setFavorited] = useState(false);
  const [favBusy, setFavBusy] = useState(false);
  const [reviews, setReviews] = useState<GameReview[]>([]);
  const [reviewTotal, setReviewTotal] = useState(0);
  const [reviewText, setReviewText] = useState('');
  const [editingReviewId, setEditingReviewId] = useState<number | null>(null);
  const [reviewBusy, setReviewBusy] = useState(false);
  const [reporting, setReporting] = useState<{ reviewId: number; reason: string } | null>(null);

  // GP-306：区服预约
  const [servers, setServers] = useState<GameServerView[]>([]);
  const [reservedIds, setReservedIds] = useState<Set<number>>(new Set());
  const [reserveBusyId, setReserveBusyId] = useState<number | null>(null);

  const load = useCallback(() => {
    if (!id) return;
    setLoading(true);
    void (async () => {
      const payload = await fetchGameById(id);
      setGame(payload.game);
      setRatingSummary(payload.rating ?? null);
      setError(null);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载详情失败');
    }).finally(() => {
      setLoading(false);
    });
  }, [id]);

  // 攻略区（M2）：独立微任务加载，失败静默不阻塞游戏详情
  const loadGuides = useCallback(() => {
    if (!id) return;
    void (async () => {
      const result = await fetchGuides({ gameId: id, page: 1, pageSize: 3 });
      setGuides(result.guides);
    })().catch(() => {
      // 攻略区是增值信息，加载失败不打扰主内容
    });
  }, [id]);

  // GP-105 平台包体：最新已发布版本（失败静默，回退商店链接）
  const loadPackage = useCallback(() => {
    if (!id) return;
    void fetchLatestGameVersion(id)
      .then(setPkg)
      .catch(() => {
        // 元数据拉取失败不阻断详情；下载区隐藏，入口区走下载/商店链接
      });
  }, [id]);

  // GP-401 长评列表（失败静默，与攻略区同策略）
  const loadReviews = useCallback(() => {
    if (!id) return;
    void (async () => {
      const list = await fetchGameReviews(id);
      setReviews(list.items);
      setReviewTotal(list.total);
    })().catch(() => {
      setReviews([]);
      setReviewTotal(0);
    });
  }, [id]);

  // GP-306 公开开服表（closed 对玩家隐藏，后端口径）
  const loadServers = useCallback(() => {
    if (!id) return;
    void fetchGameServers(id)
      .then(setServers)
      .catch(() => {
        setServers([]);
      });
  }, [id]);

  // 登录态附加加载：收藏态与我的预约（失败静默，按钮仍可用，操作以服务端为准）
  const loadAuthed = useCallback(() => {
    if (!id || !token) {
      setFavorited(false);
      setReservedIds(new Set());
      return;
    }
    void fetchMyFavoriteGameIds(token)
      .then((ids) => setFavorited(ids.has(id)))
      .catch(() => {});
    void fetchMyReservations(id, token)
      .then((mine) => setReservedIds(new Set(mine.map((r) => r.serverId))))
      .catch(() => setReservedIds(new Set()));
  }, [id, token]);

  // 发起加载推迟到微任务，effect 同步调用栈内不触发 setState（与 web-client 同款纪律）。
  useEffect(() => {
    queueMicrotask(load);
  }, [load]);

  useEffect(() => {
    queueMicrotask(loadGuides);
  }, [loadGuides]);

  useEffect(() => {
    queueMicrotask(loadPackage);
  }, [loadPackage]);

  useEffect(() => {
    queueMicrotask(loadReviews);
  }, [loadReviews]);

  useEffect(() => {
    queueMicrotask(loadServers);
  }, [loadServers]);

  useEffect(() => {
    queueMicrotask(loadAuthed);
  }, [loadAuthed]);

  const submitMyRating = useCallback(
    (score: number) => {
      if (!id || ratingBusy || !token) return;
      setRatingBusy(true);
      void submitGameRating(id, score, token)
        .then(() => setMyRating(score))
        .catch((err: unknown) => {
          Alert.alert('评分失败', err instanceof Error ? err.message : '请稍后重试');
        })
        .finally(() => setRatingBusy(false));
    },
    [id, ratingBusy, token],
  );

  const toggleFavorite = useCallback(() => {
    if (!id || favBusy || !token) return;
    setFavBusy(true);
    void (async () => {
      if (favorited) {
        await removeFavorite(id, token);
        setFavorited(false);
      } else {
        await addFavorite(id, token);
        setFavorited(true);
      }
    })()
      .catch((err: unknown) => {
        Alert.alert('收藏操作失败', err instanceof Error ? err.message : '请稍后重试');
      })
      .finally(() => setFavBusy(false));
  }, [favorited, favBusy, id, token]);

  const toggleReserve = useCallback(
    (serverId: number) => {
      if (!id || reserveBusyId !== null || !token) return;
      setReserveBusyId(serverId);
      void (async () => {
        if (reservedIds.has(serverId)) {
          await cancelReservation(id, serverId, token);
          setReservedIds((prev) => {
            const next = new Set(prev);
            next.delete(serverId);
            return next;
          });
        } else {
          const result = await reserveServer(id, serverId, token);
          setReservedIds((prev) => new Set(prev).add(serverId));
          if (!result.created) {
            Alert.alert('已预约过该区服');
          }
        }
      })()
        .catch((err: unknown) => {
          Alert.alert('预约操作失败', err instanceof Error ? err.message : '请稍后重试');
        })
        .finally(() => setReserveBusyId(null));
    },
    [id, reserveBusyId, reservedIds, token],
  );

  // 提交/改评：editingReviewId 非空走 PUT（作者覆盖式），否则 POST（重复 409 提示改评）
  const submitReview = useCallback(() => {
    if (!id || reviewBusy || !token) return;
    const content = reviewText.trim();
    setReviewBusy(true);
    void (async () => {
      if (editingReviewId !== null) {
        const updated = await updateGameReview(id, editingReviewId, content, token);
        setReviews((prev) => prev.map((r) => (r.id === updated.id ? updated : r)));
        setEditingReviewId(null);
      } else {
        const created = await createGameReview(id, content, token);
        setReviews((prev) => [created, ...prev]);
        setReviewTotal((t) => t + 1);
      }
      setReviewText('');
    })()
      .catch((err: unknown) => {
        Alert.alert('发布长评失败', err instanceof Error ? err.message : '请稍后重试');
      })
      .finally(() => setReviewBusy(false));
  }, [editingReviewId, id, reviewBusy, reviewText, token]);

  const submitReport = useCallback(() => {
    if (!id || !token || !reporting) return;
    const reason = reporting.reason.trim();
    if (!reason) return;
    void reportGameReview(id, reporting.reviewId, reason, token)
      .then(() => {
        Alert.alert('举报已提交');
        setReporting(null);
      })
      .catch((err: unknown) => {
        Alert.alert('举报失败', err instanceof Error ? err.message : '请稍后重试');
      });
  }, [id, reporting, token]);

  return (
    <View style={styles.container}>
      <View style={[styles.header, { paddingTop: insets.top + 16 }]}>
        <Pressable accessibilityLabel="返回" style={styles.back} onPress={() => router.back()} hitSlop={8}>
          <Ionicons name="chevron-back" size={20} color={colors.text} />
        </Pressable>
        <Text style={styles.headerTitle} numberOfLines={1}>
          {game?.title ?? '游戏详情'}
        </Text>
        {token && game ? (
          <Pressable
            accessibilityLabel={favorited ? '取消收藏' : '收藏'}
            style={styles.back}
            disabled={favBusy}
            hitSlop={8}
            onPress={toggleFavorite}>
            <Ionicons name={favorited ? 'star' : 'star-outline'} size={22} color={favorited ? colors.primary : colors.text} />
          </Pressable>
        ) : (
          <View style={styles.back} />
        )}
      </View>

      {loading ? (
        <View style={styles.center}>
          <ActivityIndicator color={colors.primary} size="large" />
        </View>
      ) : error || !game ? (
        <View style={styles.center}>
          <Text style={styles.errorText}>{error ?? '游戏不存在'}</Text>
          <Pressable style={styles.retryBtn} onPress={load}>
            <Text style={styles.retryText}>重试</Text>
          </Pressable>
        </View>
      ) : (
        <ScrollView contentContainerStyle={styles.content}>
          <View style={styles.hero}>
            <View style={styles.scoreBadge}>
              <Text style={styles.scoreValue}>{game.score?.toFixed(1) ?? '—'}</Text>
              <Text style={styles.scoreLabel}>资料评分</Text>
            </View>
            <View style={styles.scoreBadge}>
              <Text style={styles.scoreValue}>{ratingSummary && ratingSummary.count > 0 ? ratingSummary.average.toFixed(1) : '—'}</Text>
              <Text style={styles.scoreLabel}>玩家{ratingSummary ? ratingSummary.count : 0}</Text>
            </View>
            <View style={styles.heroMain}>
              <Text style={styles.title}>{game.title}</Text>
              <Text style={styles.metaLine}>
                {game.developer ?? '未知工作室'}
                {game.release_date ? ` · ${game.release_date}` : ''}
              </Text>
              {game.publisher ? (
                <Text style={styles.metaLine}>发行：{game.publisher}</Text>
              ) : null}
            </View>
          </View>

          {token ? (
            <View style={styles.section}>
              <Text style={styles.sectionTitle}>我的评分</Text>
              <View style={styles.starRow} accessibilityLabel="玩家评分">
                {[1, 2, 3, 4, 5].map((star) => (
                  <Pressable
                    key={star}
                    accessibilityLabel={`${star} 星`}
                    disabled={ratingBusy}
                    onPress={() => submitMyRating(star)}
                    style={styles.starBtn}>
                    <Text style={[styles.starText, myRating >= star && styles.starTextOn]}>★</Text>
                  </Pressable>
                ))}
              </View>
            </View>
          ) : null}

          {servers.length > 0 ? (
            <View style={styles.section}>
              <Text style={styles.sectionTitle}>区服与预约</Text>
              {servers.map((server) => (
                <View key={server.id} style={styles.serverRow}>
                  <View style={styles.serverInfo}>
                    <Text style={styles.serverName}>{server.name}</Text>
                    <Text style={styles.serverMeta}>
                      {server.region || '—'}
                      {server.status === 'closed' && server.plannedOpenAt
                        ? ` · 排期 ${server.plannedOpenAt.slice(0, 10)}`
                        : ''}
                    </Text>
                  </View>
                  {token ? (
                    <Pressable
                      style={styles.reserveBtn}
                      disabled={reserveBusyId === server.id}
                      onPress={() => toggleReserve(server.id)}>
                      <Text style={styles.reserveBtnText}>
                        {reserveBusyId === server.id
                          ? '处理中'
                          : reservedIds.has(server.id)
                            ? '取消预约'
                            : server.status === 'closed'
                              ? '预约开服'
                              : '预约提醒'}
                      </Text>
                    </Pressable>
                  ) : null}
                </View>
              ))}
              {!token ? (
                <Text style={styles.guideEmpty}>登录后可预约未开服区服，开服时收到提醒</Text>
              ) : null}
            </View>
          ) : null}

          {game.description ? (
            <View style={styles.section}>
              <Text style={styles.sectionTitle}>简介</Text>
              <Text style={styles.desc}>{game.description}</Text>
            </View>
          ) : null}

          {game.genres?.length ? (
            <View style={styles.section}>
              <Text style={styles.sectionTitle}>类型</Text>
              <View style={styles.chipRow}>
                {game.genres.map((genre) => (
                  <Text key={genre} style={styles.chip}>
                    {genre}
                  </Text>
                ))}
              </View>
            </View>
          ) : null}

          {game.platforms?.length ? (
            <View style={styles.section}>
              <Text style={styles.sectionTitle}>平台</Text>
              <View style={styles.chipRow}>
                {game.platforms.map((platform) => (
                  <Text key={platform} style={[styles.chip, styles.chipPlatform]}>
                    {platform}
                  </Text>
                ))}
              </View>
            </View>
          ) : null}

          {game.tags?.length ? (
            <View style={styles.section}>
              <Text style={styles.sectionTitle}>标签</Text>
              <View style={styles.chipRow}>
                {game.tags.map((tag) => (
                  <Text key={tag} style={[styles.chip, styles.chipTag]}>
                    {tag}
                  </Text>
                ))}
              </View>
            </View>
          ) : null}

          {game.play_url || game.download_url || game.store_link || pkg?.downloadable ? (
            <View style={styles.section}>
              <Text style={styles.sectionTitle}>入口</Text>
              {pkg?.downloadable ? <PackageDownload gameId={id!} pkg={pkg} /> : null}
              <View style={styles.entryRow}>
                {game.play_url ? (
                  <Pressable style={styles.entryBtn} onPress={() => Linking.openURL(game.play_url!)}>
                    <Text style={styles.entryBtnText}>进入游戏</Text>
                  </Pressable>
                ) : null}
                {game.download_url ? (
                  <Pressable style={styles.entryBtn} onPress={() => Linking.openURL(game.download_url!)}>
                    <Text style={styles.entryBtnText}>下载 Android 包</Text>
                  </Pressable>
                ) : null}
                {game.store_link ? (
                  <Pressable style={styles.entryBtn} onPress={() => Linking.openURL(game.store_link!)}>
                    <Text style={styles.entryBtnText}>应用商店</Text>
                  </Pressable>
                ) : null}
              </View>
            </View>
          ) : null}

          {game.screenshots?.length ? (
            <View style={styles.section}>
              <Text style={styles.sectionTitle}>截图</Text>
              <ScrollView horizontal showsHorizontalScrollIndicator={false}>
                {game.screenshots.map((src) => (
                  <Image key={src} source={{ uri: src }} style={styles.shot} resizeMode="cover" />
                ))}
              </ScrollView>
            </View>
          ) : null}

          {game.video_url ? (
            <View style={styles.section}>
              <Text style={styles.sectionTitle}>宣传视频</Text>
              <Pressable style={styles.entryBtn} onPress={() => Linking.openURL(game.video_url!)}>
                <Text style={styles.entryBtnText}>播放视频</Text>
              </Pressable>
            </View>
          ) : null}

          {game.changelog ? (
            <View style={styles.section}>
              <Text style={styles.sectionTitle}>更新日志</Text>
              <Text style={styles.desc}>{game.changelog}</Text>
            </View>
          ) : null}

          {game.developer_website ? (
            <View style={styles.section}>
              <Text style={styles.sectionTitle}>厂商资料</Text>
              <Pressable onPress={() => Linking.openURL(game.developer_website!)}>
                <Text style={styles.moreLink}>{game.developer ?? '厂商主页'} ›</Text>
              </Pressable>
            </View>
          ) : null}

          <View style={styles.section}>
            <View style={styles.sectionHead}>
              <Text style={styles.sectionTitle}>攻略</Text>
              <Pressable
                hitSlop={8}
                onPress={() =>
                  router.push({ pathname: '/guides', params: { game_id: id, title: game.title } })
                }>
                <Text style={styles.moreLink}>全部攻略 ›</Text>
              </Pressable>
            </View>
            {guides.length > 0 ? (
              guides.map((guide) => (
                <Pressable
                  key={guide.id}
                  onPress={() => router.push(`/guide/${guide.id}`)}
                  style={styles.guideRow}>
                  <Text style={styles.guideTitle} numberOfLines={1}>
                    {guide.title}
                  </Text>
                  <Text style={styles.guideMeta} numberOfLines={1}>
                    {guide.author_name} · {guide.views} 次阅读
                  </Text>
                </Pressable>
              ))
            ) : (
              <Text style={styles.guideEmpty}>暂无攻略，去社区看看大家的讨论</Text>
            )}
          </View>

          <View style={styles.section}>
            <Text style={styles.sectionTitle}>玩家长评{reviewTotal > 0 ? `（${reviewTotal}）` : ''}</Text>
            {token ? (
              <>
                <TextInput
                  value={reviewText}
                  onChangeText={setReviewText}
                  placeholder={editingReviewId !== null ? '修改你的长评（10-4000 字）' : '写下你的长评（10-4000 字）'}
                  placeholderTextColor={colors.textMuted}
                  multiline
                  style={styles.reviewInput}
                />
                <View style={styles.entryRow}>
                  <Pressable
                    style={styles.entryBtn}
                    disabled={reviewBusy || reviewText.trim().length < 10}
                    onPress={submitReview}>
                    <Text style={styles.entryBtnText}>
                      {reviewBusy ? '提交中…' : editingReviewId !== null ? '保存修改' : '发布长评'}
                    </Text>
                  </Pressable>
                  {editingReviewId !== null ? (
                    <Pressable
                      style={styles.entryBtn}
                      onPress={() => {
                        setEditingReviewId(null);
                        setReviewText('');
                      }}>
                      <Text style={styles.entryBtnText}>取消编辑</Text>
                    </Pressable>
                  ) : null}
                </View>
              </>
            ) : (
              <Text style={styles.guideEmpty}>登录后可写长评</Text>
            )}
            {reviews.length > 0 ? (
              reviews.map((review) => (
                <View key={review.id} style={styles.reviewCard}>
                  <View style={styles.reviewHead}>
                    <Text style={styles.reviewAuthor}>玩家 #{review.userId}</Text>
                    {review.rating > 0 ? (
                      <Text style={styles.reviewRating}>★ {review.rating}/5</Text>
                    ) : null}
                    <Text style={styles.guideMeta}>{review.updatedAt?.slice(0, 10)}</Text>
                  </View>
                  <Text style={styles.desc}>{review.content}</Text>
                  {review.devReply ? (
                    <View style={styles.devReply}>
                      <Text style={styles.devReplyLabel}>开发者回复：</Text>
                      <Text style={styles.devReplyText}>{review.devReply}</Text>
                    </View>
                  ) : null}
                  {reporting?.reviewId === review.id ? (
                    <View style={styles.reportBox}>
                      <TextInput
                        value={reporting.reason}
                        onChangeText={(reason) => setReporting({ reviewId: review.id, reason })}
                        placeholder="举报理由（1-500 字）"
                        placeholderTextColor={colors.textMuted}
                        multiline
                        style={styles.reviewInput}
                      />
                      <View style={styles.entryRow}>
                        <Pressable style={styles.entryBtn} onPress={submitReport}>
                          <Text style={styles.entryBtnText}>提交举报</Text>
                        </Pressable>
                        <Pressable style={styles.entryBtn} onPress={() => setReporting(null)}>
                          <Text style={styles.entryBtnText}>取消</Text>
                        </Pressable>
                      </View>
                    </View>
                  ) : (
                    <View style={styles.entryRow}>
                      {user && review.userId === user.id ? (
                        <Pressable
                          style={styles.entryBtn}
                          onPress={() => {
                            setEditingReviewId(review.id);
                            setReviewText(review.content);
                          }}>
                          <Text style={styles.entryBtnText}>编辑</Text>
                        </Pressable>
                      ) : null}
                      {token ? (
                        <Pressable
                          style={styles.entryBtn}
                          onPress={() => setReporting({ reviewId: review.id, reason: '' })}>
                          <Text style={styles.entryBtnText}>举报</Text>
                        </Pressable>
                      ) : null}
                    </View>
                  )}
                </View>
              ))
            ) : (
              <Text style={styles.guideEmpty}>暂无长评，来写第一条</Text>
            )}
          </View>

          <View style={styles.section}>
            <Text style={styles.sectionTitle}>热度</Text>
            <Text style={styles.trending}>🔥 trending score {game.trending_score ?? 0}</Text>
          </View>
        </ScrollView>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.background,
  },
  header: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 16,
    paddingBottom: 8,
  },
  back: {
    minWidth: 44,
    minHeight: 44,
    alignItems: 'center',
    justifyContent: 'center',
    width: 32,
  },
  headerTitle: {
    flex: 1,
    textAlign: 'center',
    color: colors.text,
    fontSize: 16,
    fontWeight: '600',
  },
  center: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    gap: 12,
    padding: 24,
  },
  errorText: {
    color: colors.danger,
    fontSize: 14,
  },
  retryBtn: {
    minHeight: 44,
    alignItems: 'center',
    justifyContent: 'center',
    borderWidth: 1,
    borderColor: colors.primary,
    borderRadius: 8,
    paddingHorizontal: 16,
    paddingVertical: 8,
  },
  retryText: {
    color: colors.primary,
    fontSize: 13,
  },
  content: {
    padding: 16,
    paddingBottom: 40,
    gap: 16,
  },
  hero: {
    flexDirection: 'row',
    gap: 16,
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 16,
  },
  scoreBadge: {
    width: 72,
    height: 72,
    borderRadius: 16,
    backgroundColor: colors.background,
    borderWidth: 1,
    borderColor: colors.border,
    alignItems: 'center',
    justifyContent: 'center',
  },
  scoreValue: {
    color: colors.primary,
    fontSize: 22,
    fontWeight: '800',
  },
  scoreLabel: {
    color: colors.textMuted,
    fontSize: 11,
  },
  heroMain: {
    flex: 1,
    gap: 6,
    justifyContent: 'center',
  },
  title: {
    color: colors.text,
    fontSize: 20,
    fontWeight: '700',
  },
  metaLine: {
    color: colors.textMuted,
    fontSize: 13,
  },
  section: {
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 16,
    gap: 10,
  },
  sectionTitle: {
    color: colors.text,
    fontSize: 15,
    fontWeight: '600',
  },
  sectionHead: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  moreLink: {
    color: colors.primary,
    fontSize: 13,
  },
  guideRow: {
    gap: 2,
    paddingVertical: 4,
  },
  guideTitle: {
    color: colors.text,
    fontSize: 14,
  },
  guideMeta: {
    color: colors.textMuted,
    fontSize: 12,
  },
  guideEmpty: {
    color: colors.textMuted,
    fontSize: 13,
  },
  desc: {
    color: colors.textMuted,
    fontSize: 14,
    lineHeight: 22,
  },
  chipRow: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 8,
  },
  chip: {
    minHeight: 44,
    alignItems: 'center',
    justifyContent: 'center',
    color: colors.text,
    backgroundColor: colors.background,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 10,
    paddingVertical: 4,
    fontSize: 12,
    overflow: 'hidden',
  },
  chipPlatform: {
    color: colors.textMuted,
  },
  entryRow: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 8,
  },
  entryBtn: {
    minHeight: 44,
    alignItems: 'center',
    justifyContent: 'center',
    borderWidth: 1,
    borderColor: colors.primary,
    borderRadius: 8,
    paddingHorizontal: 14,
    paddingVertical: 8,
  },
  entryBtnText: {
    color: colors.primary,
    fontSize: 13,
  },
  shot: {
    width: 200,
    height: 112,
    borderRadius: 8,
    marginRight: 8,
    backgroundColor: colors.background,
  },
  chipTag: {
    borderColor: colors.primary,
    color: colors.primary,
  },
  trending: {
    color: colors.textMuted,
    fontSize: 13,
  },
  // GP-401 评分/长评/收藏 + GP-306 区服预约
  starRow: {
    flexDirection: 'row',
    gap: 4,
  },
  starBtn: {
    minWidth: 44,
    minHeight: 44,
    alignItems: 'center',
    justifyContent: 'center',
  },
  starText: {
    fontSize: 26,
    lineHeight: 30,
    color: colors.border,
  },
  starTextOn: {
    color: colors.primary,
  },
  serverRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
    paddingVertical: 4,
  },
  serverInfo: {
    flex: 1,
    gap: 2,
  },
  serverName: {
    color: colors.text,
    fontSize: 14,
  },
  serverMeta: {
    color: colors.textMuted,
    fontSize: 12,
  },
  reserveBtn: {
    minHeight: 44,
    alignItems: 'center',
    justifyContent: 'center',
    borderWidth: 1,
    borderColor: colors.primary,
    borderRadius: 8,
    paddingHorizontal: 12,
  },
  reserveBtnText: {
    color: colors.primary,
    fontSize: 13,
  },
  reviewInput: {
    color: colors.text,
    backgroundColor: colors.background,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: 8,
    padding: 10,
    minHeight: 80,
    fontSize: 14,
    textAlignVertical: 'top',
  },
  reviewCard: {
    gap: 6,
    paddingVertical: 8,
    borderTopWidth: 1,
    borderTopColor: colors.border,
  },
  reviewHead: {
    flexDirection: 'row',
    alignItems: 'baseline',
    gap: 10,
  },
  reviewAuthor: {
    color: colors.text,
    fontSize: 13,
  },
  reviewRating: {
    color: colors.primary,
    fontSize: 12,
  },
  devReply: {
    borderLeftWidth: 2,
    borderLeftColor: colors.primary,
    backgroundColor: colors.background,
    padding: 8,
    borderRadius: 4,
  },
  devReplyLabel: {
    color: colors.textMuted,
    fontSize: 12,
  },
  devReplyText: {
    color: colors.textMuted,
    fontSize: 12,
    lineHeight: 18,
  },
  reportBox: {
    gap: 8,
  },
});
