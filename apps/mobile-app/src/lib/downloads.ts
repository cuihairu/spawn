// GP-105 Android 下载队列：包体受控下载的客户端管理。
//
// 设计口径：
// - 走 gateway 流式透传通道 GET /games/:id/versions/:versionId/download，
//   带 Bearer 头（服务端只放行已发布+平台托管版本）。
// - 暂停/续传用 expo-file-system 新版 DownloadTask（pauseAsync/resumeAsync，
//   原生断点续传）；跨会话续传：暂停时 task.savable() 落盘
//   （resume-<文件名>.json），重启后 DownloadTask.fromSavable 重建暂停态
//   任务再 resumeAsync（见 loadResumable/createResumablePackageDownload）。
// - 完整性校验：下载完成后比对 Content-Length 与 size_bytes（截断检测），
//   再比对服务端登记的 SHA-256（原生流式哈希，见
//   modules/spawn-file-digest）；expo-crypto 仅支持整块 digest（无流式/
//   文件接口，2 GiB 包体读入内存不可行），故自研本地 Expo Module。
// - 安装：Android 经 IntentLauncher ACTION_VIEW 打开 APK（需要用户允许
//   未知来源安装，REQ_INSTALL_PACKAGES 权限在 app.json 声明）。

import { Directory, File, Paths, DownloadTask, type DownloadPauseState, type DownloadTaskOptions } from 'expo-file-system';
import { getContentUriAsync } from 'expo-file-system/legacy';
import * as IntentLauncher from 'expo-intent-launcher';
import { Platform } from 'react-native';

import { digestFileSha256Async } from '../../modules/spawn-file-digest';

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
  /** 服务端登记的 SHA-256（小写 hex，无 sha256: 前缀）；下载完成后流式比对 */
  expectedSha256?: string;
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

/** 跨会话续传的落盘快照（暂停时写入，恢复/取消/完成后清除） */
export interface SavedDownloadState {
  url: string;
  fileName: string;
  expectedSize?: number;
  savedAt: number;
  pauseState: DownloadPauseState;
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

function resumeStateFile(fileName: string): File {
  return new File(downloadsDir(), `resume-${safeFileName(fileName)}.json`);
}

/** 读取某包体的跨会话续传快照（无/损坏/字段缺失 → null） */
export function loadResumable(fileName: string): SavedDownloadState | null {
  const stateFile = resumeStateFile(fileName);
  if (!stateFile.exists) {
    return null;
  }
  try {
    const saved = JSON.parse(stateFile.textSync()) as SavedDownloadState;
    if (!saved || typeof saved.url !== 'string' || !saved.pauseState || typeof saved.pauseState.url !== 'string') {
      return null;
    }
    return saved;
  } catch {
    return null;
  }
}

function clearResumable(fileName: string): void {
  try {
    const stateFile = resumeStateFile(fileName);
    if (stateFile.exists) {
      stateFile.delete();
    }
  } catch {
    // 清理失败不影响主流程
  }
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
    // 上次失败/取消的残留：从头重来（跨会话续传快照一并清除）
    if (this.file.exists) {
      this.file.delete();
    }
    clearResumable(this.opts.fileName);
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
      // 跨会话续传：savable 快照落盘（仅暂停态可取；失败不阻断暂停）
      try {
        const saved: SavedDownloadState = {
          url: this.opts.url,
          fileName: this.opts.fileName,
          expectedSize: this.opts.expectedSize,
          savedAt: Date.now(),
          pauseState: this.task.savable(),
        };
        resumeStateFile(this.opts.fileName).write(JSON.stringify(saved));
      } catch {
        // 落盘失败退化为会话内续传
      }
      this.emit();
    }
  }

  async resume(): Promise<void> {
    if (this.task && this.phase === 'paused') {
      this.phase = 'downloading';
      this.emit();
      // 真恢复：native 断点续传（原实现误调 run() 重开新任务，丢进度；
      // P1-4 修复）。resumeAsync 本身 await 完成（完成解析为 File，
      // 再暂停解析 null），完成后的校验/落库与 start 共用 onFinished。
      const result = await this.task.resumeAsync();
      await this.onFinished(result);
    }
  }

  cancel(): void {
    this.cancelled = true;
    this.task?.cancel();
    this.phase = 'idle';
    this.bytesWritten = 0;
    clearResumable(this.opts.fileName);
    this.emit();
  }

  /** 进度回调/头注入（start 与 fromSavable 重建共用；savable 不序列化回调） */
  private progressOptions(): DownloadTaskOptions {
    return {
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
  }

  private async run(dest: File): Promise<void> {
    this.phase = 'downloading';
    this.emit();
    this.task = File.createDownloadTask(this.opts.url, dest, this.progressOptions());
    try {
      const result = await this.task.downloadAsync();
      await this.onFinished(result);
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

  // restore 由落盘快照重建暂停态任务（跨会话续传入口）：目标文件与
  // 暂停态保留，回调经 progressOptions 重挂（新头覆盖旧 Authorization）。
  static restore(saved: SavedDownloadState, opts: PackageDownloadOptions): PackageDownloadHandle {
    const inst = new PackageDownload(opts);
    inst.file = new File(downloadsDir(), safeFileName(saved.fileName));
    inst.task = DownloadTask.fromSavable(saved.pauseState, inst.progressOptions());
    inst.phase = 'paused';
    return inst;
  }

  // onFinished 处理下载完成结果：null = 被暂停（phase 由 pause() 置
  // paused）；否则截断校验 + 落 done。start 与 resume 共用（P1-4）。
  private async onFinished(result: File | null): Promise<void> {
    if (result === null) {
      return;
    }
    // 下载完成：跨会话续传快照已失效
    clearResumable(this.opts.fileName);
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
    // 完整性校验（GP-105）：服务端登记了 SHA-256 时，用原生流式哈希比对。
    // 原生模块不可用（开发期未 prebuild 生成原生工程）时不阻断，退化为仅
    // 大小校验——生产构建必含该模块，哈希不一致视为损坏。
    const expectedHash = this.opts.expectedSha256?.trim().toLowerCase();
    if (expectedHash && /^[0-9a-f]{64}$/.test(expectedHash)) {
      try {
        const actualHash = await digestFileSha256Async(result.uri);
        if (actualHash.toLowerCase() !== expectedHash) {
          this.error = '包体哈希校验失败（SHA-256 与服务端登记不一致）';
          this.phase = 'error';
          try {
            result.delete();
          } catch {
            // 清理失败不阻断错误上报
          }
          this.emit();
          return;
        }
      } catch (err) {
        // 模块缺失/文件不可读：仅记录，不阻断（大小校验已过）
        console.warn('SHA-256 校验跳过：', err);
      }
    }
    this.bytesWritten = actual ?? 0;
    this.totalBytes = actual ?? 0;
    this.file = result;
    this.phase = 'done';
    this.emit();
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

/**
 * 由落盘快照重建暂停态下载（App 重启后的跨会话续传）。
 * snapshot 为 paused 相位；续传走既有 resume()（fromSavable 任务 +
 * 新注入的头，Authorization 以本次登录态为准）。
 */
export function createResumablePackageDownload(
  saved: SavedDownloadState,
  opts: PackageDownloadOptions,
): PackageDownloadHandle {
  return PackageDownload.restore(saved, opts);
}
