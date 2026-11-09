// GP-105 Android 下载队列：包体受控下载的客户端管理。
//
// 设计口径：
// - 走 gateway 流式透传通道 GET /games/:id/versions/:versionId/download，
//   带 Bearer 头（服务端只放行已发布+平台托管版本）。
// - 暂停/续传用 expo-file-system 新版 DownloadTask（pauseAsync/resumeAsync，
//   原生断点续传）；本次会话内恢复，App 重启后的跨会话续传留待原生补齐
//   （DownloadPauseState.savable() 已可序列化，此处先不落盘）。
// - 完整性校验：下载完成后比对 Content-Length 与 size_bytes（截断检测）。
//   SHA-256 校验（X-Package-Sha256）需原生文件哈希模块，JS 侧无逐块哈希
//   能力，作为已知缺口记录（真机验收时以 X-Package-Sha256 比对）。
// - 安装：Android 经 IntentLauncher ACTION_VIEW 打开 APK（需要用户允许
//   未知来源安装，REQ_INSTALL_PACKAGES 权限在 app.json 声明）。

import { Directory, File, Paths, DownloadTask, type DownloadTaskOptions } from 'expo-file-system';
import { getContentUriAsync } from 'expo-file-system/legacy';
import * as IntentLauncher from 'expo-intent-launcher';
import { Platform } from 'react-native';

export type DownloadPhase = 'idle' | 'downloading' | 'paused' | 'verifying' | 'done' | 'error';

export interface DownloadSnapshot {
  phase: DownloadPhase;
  /** 0..1，totalBytes 未知时按已写字节推进（unknown 时用大小头估算） */
  progress: number;
  bytesWritten: number;
  totalBytes: number;
  error?: string;
  fileUri?: string;
  fileName?: string;
}

export interface PackageDownloadOptions {
  url: string;
  fileName: string;
  /** 服务端 size_bytes；下载完成后的截断校验基准 */
  expectedSize?: number;
  headers?: Record<string, string>;
  onUpdate: (snapshot: DownloadSnapshot) => void;
}

export interface PackageDownloadHandle {
  start(): Promise<void>;
  pause(): Promise<void>;
  resume(): Promise<void>;
  retry(): Promise<void>;
  cancel(): void;
  /** Android：已下载 APK 调系统安装界面 */
  install(): Promise<void>;
  get snapshot(): DownloadSnapshot;
}

const DL_DIR = 'spawn-downloads';

function downloadsDir(): Directory {
  const dir = new Directory(Paths.cache, DL_DIR);
  if (!dir.exists) {
    dir.create({ intermediates: true });
  }
  return dir;
}

function safeFileName(name: string): string {
  // 只留字母数字._-，避免路径穿越
  const cleaned = name.replace(/[^A-Za-z0-9._-]/g, '_').slice(0, 128);
  return cleaned || `pkg-${Date.now()}.apk`;
}

class PackageDownload implements PackageDownloadHandle {
  private opts: PackageDownloadOptions;
  private task: DownloadTask | null = null;
  private file: File | null = null;
  private phase: DownloadPhase = 'idle';
  private bytesWritten = 0;
  private totalBytes = 0;
  private error?: string;
  private cancelled = false;

  constructor(opts: PackageDownloadOptions) {
    this.opts = opts;
  }

  get snapshot(): DownloadSnapshot {
    const progress = this.totalBytes > 0
      ? Math.min(1, this.bytesWritten / this.totalBytes)
      : 0;
    return {
      phase: this.phase,
      progress,
      bytesWritten: this.bytesWritten,
      totalBytes: this.totalBytes,
      error: this.error,
      fileUri: this.file?.uri,
      fileName: this.file ? this.file.name : undefined,
    };
  }

  private emit(): void {
    this.opts.onUpdate(this.snapshot);
  }

  async start(): Promise<void> {
    if (this.phase !== 'idle' && this.phase !== 'error') {
      return;
    }
    const name = safeFileName(this.opts.fileName);
    this.file = new File(downloadsDir(), name);
    // 上次失败/取消的残留：从头重来
    if (this.file.exists) {
      this.file.delete();
    }
    this.error = undefined;
    await this.run(this.file);
  }

  async retry(): Promise<void> {
    await this.start();
  }

  async pause(): Promise<void> {
    if (this.task && this.phase === 'downloading') {
      await this.task.pauseAsync();
      this.phase = 'paused';
      this.emit();
    }
  }

  async resume(): Promise<void> {
    if (this.task && this.phase === 'paused') {
      this.phase = 'downloading';
      this.emit();
      await this.run(this.file!, /* resumed */ true);
    }
  }

  cancel(): void {
    this.cancelled = true;
    this.task?.cancel();
    this.phase = 'idle';
    this.bytesWritten = 0;
    this.emit();
  }

  private async run(dest: File, resumed = false): Promise<void> {
    const options: DownloadTaskOptions = {
      headers: this.opts.headers,
      onProgress: (p) => {
        this.bytesWritten = p.bytesWritten;
        this.totalBytes = p.totalBytes > 0 ? p.totalBytes : this.opts.expectedSize ?? 0;
        if (this.phase !== 'paused') {
          this.phase = 'downloading';
        }
        this.emit();
      },
    };
    this.phase = 'downloading';
    this.emit();
    this.task = File.createDownloadTask(this.opts.url, dest, options);
    try {
      const result = await this.task.downloadAsync();
      if (result === null) {
        // 被暂停：downloadAsync 返回 null，phase 由 pause() 置为 paused
        return;
      }
      this.phase = 'verifying';
      this.emit();
      // 截断校验：字节数必须等于服务端登记的 size_bytes（GP-104 登记值）
      const actual = result.size;
      if (typeof this.opts.expectedSize === 'number' && this.opts.expectedSize > 0 && actual !== this.opts.expectedSize) {
        this.error = `包体大小校验失败（${actual} ≠ ${this.opts.expectedSize} 字节）`;
        this.phase = 'error';
        try {
          result.delete();
        } catch {
          // 清理失败不阻断错误上报
        }
        this.emit();
        return;
      }
      this.bytesWritten = actual ?? 0;
      this.totalBytes = actual ?? 0;
      this.file = result;
      this.phase = 'done';
      this.emit();
    } catch (err) {
      // 主动取消不进入 error（cancel() 已复位 phase）
      if (this.cancelled) {
        return;
      }
      this.error = err instanceof Error ? err.message : '下载失败';
      this.phase = 'error';
      this.emit();
    }
  }

  async install(): Promise<void> {
    if (this.phase !== 'done' || !this.file || Platform.OS !== 'android') {
      return;
    }
    // 经 FileProvider 转 content:// 供系统安装器读（跨应用权限）
    const contentUri = await getContentUriAsync(this.file.uri);
    await IntentLauncher.startActivityAsync('android.intent.action.VIEW', {
      data: contentUri,
      type: 'application/vnd.android.package-archive',
      // NEW_TASK | GRANT_READ_URI_PERMISSION
      flags: 0x10000000 | 0x00000001,
    });
  }
}

export function createPackageDownload(opts: PackageDownloadOptions): PackageDownloadHandle {
  return new PackageDownload(opts);
}
