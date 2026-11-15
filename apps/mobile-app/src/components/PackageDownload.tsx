// GP-105 平台包体下载区：受控下载（登录）→ 进度/暂停/续传/重试 → 大小校验 → 安装。
// 服务端只放行「已发布 + 平台托管」版本；下载通道经网关流式透传（GET
// /games/:id/versions/:versionId/download），Bearer 头由本组件注入。
// 跨会话续传：挂载时读取暂停快照（resume-*.json）重建暂停态任务。
// 非 Android 不做应用内 APK 下载，暂以适用商店链接承接（入口区，F08）。

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Platform, Pressable, StyleSheet, Text, View } from 'react-native';
import { router } from 'expo-router';

import { GAME_SERVICE_URL, type LatestGameVersion } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import {
  createPackageDownload,
  createResumablePackageDownload,
  loadResumable,
  type DownloadSnapshot,
  type PackageDownloadHandle,
} from '../lib/downloads';
import { colors } from '../constants/colors';

function fmtBytes(n?: number): string {
  if (!n || n <= 0) return '未知大小';
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

export function PackageDownload({ gameId, pkg }: { gameId: string; pkg: LatestGameVersion }) {
  const { token } = useAuth();
  const [snap, setSnap] = useState<DownloadSnapshot | null>(null);
  const handleRef = useRef<PackageDownloadHandle | null>(null);

  const fileName = useMemo(
    () => `${pkg.package_name || `game-${gameId}`}-${pkg.version}-${pkg.id}.apk`,
    [gameId, pkg],
  );

  const downloadUrl = useMemo(
    () => `${GAME_SERVICE_URL}/games/${encodeURIComponent(gameId)}/versions/${encodeURIComponent(pkg.id)}/download`,
    [gameId, pkg],
  );

  const ensureHandle = useCallback((): PackageDownloadHandle => {
    if (!handleRef.current) {
      handleRef.current = createPackageDownload({
        url: downloadUrl,
        fileName,
        expectedSize: pkg.size_bytes,
        headers: token ? { Authorization: `Bearer ${token}` } : undefined,
        onUpdate: setSnap,
      });
    }
    return handleRef.current;
  }, [downloadUrl, fileName, pkg.size_bytes, token]);

  // 跨会话续传：挂载时恢复上次暂停的下载（快照 URL 与当前版本一致才恢复；
  // 已有句柄不覆盖）。重建任务为 paused 相位，续传走既有 resume()。
  useEffect(() => {
    if (handleRef.current || Platform.OS !== 'android') {
      return;
    }
    const saved = loadResumable(fileName);
    if (!saved || saved.url !== downloadUrl) {
      return;
    }
    handleRef.current = createResumablePackageDownload(saved, {
      url: downloadUrl,
      fileName,
      expectedSize: pkg.size_bytes,
      headers: token ? { Authorization: `Bearer ${token}` } : undefined,
      onUpdate: setSnap,
    });
    setSnap(handleRef.current.snapshot);
  }, [fileName, downloadUrl, pkg.size_bytes, token]);

  const phase = snap?.phase ?? 'idle';

  const onPrimary = useCallback(() => {
    if (!token) {
      router.push('/login');
      return;
    }
    const handle = ensureHandle();
    if (phase === 'idle' || phase === 'error') void handle.start();
    else if (phase === 'paused') void handle.resume();
    else if (phase === 'downloading') void handle.pause();
    else if (phase === 'done' && Platform.OS === 'android') void handle.install();
  }, [token, phase, ensureHandle]);

  const onCancel = useCallback(() => {
    handleRef.current?.cancel();
    setSnap(null);
  }, []);

  const percent = snap ? Math.round(snap.progress * 100) : 0;

  // 非 Android：不做应用内 APK 下载，暂以适用商店链接承接（入口区按钮）
  if (Platform.OS !== 'android') {
    return (
      <View style={styles.wrap}>
        <Text style={styles.meta}>
          平台安装包 v{pkg.version}
          {pkg.version_code ? ` · build ${pkg.version_code}` : ''} · {fmtBytes(pkg.size_bytes)}
        </Text>
        <Text style={styles.hint}>该平台暂不提供应用内下载，请通过应用商店链接安装。</Text>
      </View>
    );
  }
  const primaryLabel =
    phase === 'idle'
      ? `下载安装包（${fmtBytes(pkg.size_bytes)}）`
      : phase === 'downloading'
        ? '暂停'
        : phase === 'paused'
          ? `继续下载 ${percent}%`
          : phase === 'verifying'
            ? '校验中…'
            : phase === 'done'
              ? Platform.OS === 'android'
                ? '安装'
                : '已下载'
              : '重试';

  return (
    <View style={styles.wrap}>
      <Text style={styles.meta}>
        平台安装包 v{pkg.version}
        {pkg.version_code ? ` · build ${pkg.version_code}` : ''} · {fmtBytes(pkg.size_bytes)}
      </Text>

      {(phase === 'downloading' || phase === 'verifying' || phase === 'paused') && (
        <View style={styles.track}>
          <View style={[styles.fill, { width: `${Math.max(4, percent)}%` }]} />
        </View>
      )}

      {phase === 'downloading' && snap ? (
        <Text style={styles.progressText}>
          {fmtBytes(snap.bytesWritten)} / {fmtBytes(snap.totalBytes)}（{percent}%）
        </Text>
      ) : null}

      {phase === 'error' && snap?.error ? <Text style={styles.error}>{snap.error}</Text> : null}

      <View style={styles.row}>
        <Pressable
          accessibilityLabel={primaryLabel}
          style={[styles.btn, phase === 'done' && Platform.OS !== 'android' ? styles.btnDisabled : null]}
          disabled={phase === 'done' && Platform.OS !== 'android'}
          onPress={onPrimary}
        >
          <Text style={styles.btnText}>{primaryLabel}</Text>
        </Pressable>
        {(phase === 'downloading' || phase === 'paused' || phase === 'error') && (
          <Pressable accessibilityLabel="取消下载" style={styles.cancelBtn} onPress={onCancel}>
            <Text style={styles.cancelText}>取消</Text>
          </Pressable>
        )}
      </View>

      {phase === 'done' && Platform.OS === 'android' ? (
        <Text style={styles.hint}>若系统询问「未知来源应用」，需允许后完成安装。</Text>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { marginTop: 12 },
  meta: { color: colors.textMuted, fontSize: 12, marginBottom: 8 },
  track: {
    height: 6,
    borderRadius: 3,
    backgroundColor: colors.border,
    overflow: 'hidden',
    marginBottom: 6,
  },
  fill: { height: 6, backgroundColor: colors.primary },
  progressText: { color: colors.textMuted, fontSize: 12, marginBottom: 8 },
  error: { color: colors.danger, fontSize: 12, marginBottom: 8 },
  row: { flexDirection: 'row', alignItems: 'center', gap: 8 },
  btn: {
    paddingVertical: 10,
    paddingHorizontal: 16,
    borderRadius: 8,
    backgroundColor: colors.primary,
  },
  btnDisabled: { backgroundColor: colors.border },
  btnText: { color: '#fff', fontWeight: '600' },
  cancelBtn: { paddingVertical: 10, paddingHorizontal: 14 },
  cancelText: { color: colors.textMuted },
  hint: { color: colors.textMuted, fontSize: 11, marginTop: 8 },
});
