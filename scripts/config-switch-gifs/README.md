# config-switch-gifs

DOC-GIF：spawn 各服务配置开关的效果动态图流水线（kode C-24 同款模式）。

- `switches.json` — 唯一事实源：15 个运行期开关（8 个 capture=true 出图，7 个 capture=false 进手工验证清单）。
- `generate.py` — 逐开关对真实服务采集：临时 SQLite 库上按 off/on 各起一次服务二进制，发同样的真实 HTTP 请求，行为断言（403→200 / 200→400 / 回调数 0→1）不符即整条失败；PIL 渲染终端帧 + ffmpeg 缝循环 GIF；产物落 `docs/public/config-switch-gifs/`，契约写 `manifest.json`。
- `render_docs.py` — 从 switches.json 生成双语文档页 `docs/guide/config-effects.md` 与 `docs/zh/guide/config-effects.md`（产物页勿手改）。

```bash
python3 scripts/config-switch-gifs/generate.py --list    # 看开关清单
python3 scripts/config-switch-gifs/generate.py --all     # 全量采集（可 --switch=<key> 单跑）
python3 scripts/config-switch-gifs/render_docs.py        # 重生成双语文档页
python3 scripts/config-switch-gifs/generate.py --check   # 一致性闸（CI 可跑）
```

口径（ kode C-24 同款）：

1. 只产真实证据：每帧文案来自真实 HTTP 响应；断言不过 = 效果未生效 = 失败，宁可不产图不出假图。
2. 帧差闸：off 与 on 末帧逐字节相同即失败（几乎一致说明开关效果不可观测）。
3. capture=false 的开关（多服务联动/时间轴/部署面效果）不配图，只进文档页文末手工验证清单。
4. manifest.json 路径一律仓库相对路径。

依赖：Go 工具链、ffmpeg、Python PIL（`fonts-noto-cjk` 提供 CJK 字形）。
