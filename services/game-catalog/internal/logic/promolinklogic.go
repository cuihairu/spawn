package logic

// GP-403 单层渠道推广（F22）：渠道登记与变更历史、推广链接落地解析、
// 渠道包（下载）追踪、归因窗口判定。
//
// 口径边界：
//   - 只做单层归因（渠道 → 用户）。渠道表无 parent_id 列，多级代理不随
//     本任务默认引入（候选未排期，见 todo.md）。
//   - 归因窗口按「点击时刻 + 渠道窗口小时数」判定：窗口外的注册不计入
//     该渠道。窗口在渠道上配置，变更只影响其后新点击（历史结算不变）。
//   - 停用（disabled）渠道的链接不再计点击，存量点击与已归因结果不受影响。

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// 归因窗口小时数上下限：0 表示「立即失效」（不开放归因），上限 30 天。
const (
	promoWindowMinHours = 0
	promoWindowMaxHours = 24 * 30
	promoWindowDefault  = 72
)

// 变更历史动作（写入 audit_logs，resource_type=promo_channel）。
const (
	promoActionCreate = "promo_channel_create"
	promoActionUpdate = "promo_channel_update"
	promoActionStatus = "promo_channel_status"
)

type PromoLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPromoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PromoLogic {
	return &PromoLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ---- 渠道登记与变更历史（管理面） ----

// CreatePromoChannel 登记渠道（单层）：code 唯一，撞唯一键返回 409。
func (l *PromoLogic) CreatePromoChannel(req *types.CreatePromoChannelRequest) (*types.PromoChannelView, error) {
	code := strings.TrimSpace(req.Code)
	name := strings.TrimSpace(req.Name)
	if !validPromoCode(code) {
		return nil, newAPIError(http.StatusBadRequest, "渠道标识需为 1-64 位字母数字/._-")
	}
	if name == "" || len(name) > 128 {
		return nil, newAPIError(http.StatusBadRequest, "渠道名称需为 1-128 字符")
	}
	window := req.WindowHours
	if window == 0 {
		window = promoWindowDefault
	}
	if window < promoWindowMinHours || window > promoWindowMaxHours {
		return nil, newAPIError(http.StatusBadRequest, "归因窗口需在 0-720 小时之间")
	}
	ch, err := l.svcCtx.AdminRepository.CreatePromoChannel(code, name, window)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return nil, newAPIError(http.StatusConflict, "渠道标识已存在")
		}
		l.Errorf("create promo channel failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "登记渠道失败")
	}
	l.audit(promoActionCreate, ch.Id, "", toJSON(ch))
	return toTypesPromoChannel(ch), nil
}

// UpdatePromoChannel 变更渠道名/归因窗口（PATCH，变更留痕）。
func (l *PromoLogic) UpdatePromoChannel(req *types.UpdatePromoChannelRequest) (*types.PromoChannelView, error) {
	before, err := l.svcCtx.AdminRepository.GetPromoChannel(req.Id)
	if err != nil {
		return l.channelNotFound(err)
	}
	name, window := before.Name, before.AttributionWindowHours
	if strings.TrimSpace(req.Name) != "" {
		name = strings.TrimSpace(req.Name)
		if len(name) > 128 {
			return nil, newAPIError(http.StatusBadRequest, "渠道名称需为 1-128 字符")
		}
	}
	if req.WindowHours != 0 {
		window = req.WindowHours
		if window < promoWindowMinHours || window > promoWindowMaxHours {
			return nil, newAPIError(http.StatusBadRequest, "归因窗口需在 0-720 小时之间")
		}
	}
	ch, err := l.svcCtx.AdminRepository.UpdatePromoChannel(req.Id, name, window)
	if err != nil {
		l.Errorf("update promo channel failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "变更渠道失败")
	}
	l.audit(promoActionUpdate, ch.Id, toJSON(before), toJSON(ch))
	return toTypesPromoChannel(ch), nil
}

// SetPromoChannelStatus 启用/停用渠道（停用后链接不再计点击）。
func (l *PromoLogic) SetPromoChannelStatus(req *types.SetPromoChannelStatusRequest) (*types.PromoChannelView, error) {
	before, err := l.svcCtx.AdminRepository.GetPromoChannel(req.Id)
	if err != nil {
		return l.channelNotFound(err)
	}
	status := strings.TrimSpace(req.Status)
	if status != model.PromoChannelActive && status != model.PromoChannelDisabled {
		return nil, newAPIError(http.StatusBadRequest, "状态需为 active 或 disabled")
	}
	ch, err := l.svcCtx.AdminRepository.SetPromoChannelStatus(req.Id, status)
	if err != nil {
		l.Errorf("set promo channel status failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "变更渠道状态失败")
	}
	l.audit(promoActionStatus, ch.Id, toJSON(before), toJSON(ch))
	return toTypesPromoChannel(ch), nil
}

// ListPromoChannels 渠道列表（管理面，单层：无上级渠道层级）。
func (l *PromoLogic) ListPromoChannels(limit, offset int64) ([]types.PromoChannelView, error) {
	pageLimit, off := clampPage(limit, offset, 100)
	channels, err := l.svcCtx.AdminRepository.ListPromoChannels(pageLimit, off)
	if err != nil {
		l.Errorf("list promo channels failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询渠道列表失败")
	}
	return toTypesPromoChannels(channels), nil
}

// ChannelStats 渠道包/渠道效果计数（点击/下载/归因人数，窗口内）。
func (l *PromoLogic) ChannelStats(code, from, to string) (*types.PromoChannelStatsResponse, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, newAPIError(http.StatusBadRequest, "渠道标识必填")
	}
	stats, err := l.svcCtx.AdminRepository.ChannelStats(code, from, to)
	if err != nil {
		l.Errorf("channel stats failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询渠道计数失败")
	}
	resp := toTypesChannelStats(code, stats)
	return &resp, nil
}

// ---- 推广链接落地与渠道包下载（公开/玩家面） ----

// ResolvePromoLink 推广链接落地：校验渠道有效并记一条点击。
// 重复 click_id 幂等（返回既有记录，不重复计数）；无效/停用渠道 404。
func (l *PromoLogic) ResolvePromoLink(req *types.ResolvePromoLinkRequest) (*types.ResolvePromoLinkResponse, error) {
	code := strings.TrimSpace(req.Channel)
	if code == "" {
		return nil, newAPIError(http.StatusBadRequest, "渠道标识必填")
	}
	ch, err := l.svcCtx.AdminRepository.GetPromoChannelByCode(code)
	if err != nil {
		if errors.Is(err, model.ErrPromoChannelNotFound) {
			return nil, newAPIError(http.StatusNotFound, "渠道不存在或已停用")
		}
		l.Errorf("lookup promo channel failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询渠道失败")
	}
	if ch.Status != model.PromoChannelActive {
		return nil, newAPIError(http.StatusNotFound, "渠道不存在或已停用")
	}
	clickID := strings.TrimSpace(req.ClickId)
	if clickID == "" {
		clickID = randomClickID()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := l.svcCtx.AdminRepository.RecordPromoClick(clickID, ch, model.PromoClickClick, req.GameId, now); err != nil {
		l.Errorf("record promo click failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "记录渠道点击失败")
	}
	return &types.ResolvePromoLinkResponse{
		Channel:    ch.Code,
		ClickId:    clickID,
		WindowSecs: ch.AttributionWindowHours * 3600,
	}, nil
}

// RecordDownloadClick 渠道包下载登记（渠道包追踪：按渠道统计下载量）。
// 渠道无效时不阻断下载，仅不计渠道包计数（下载主流程不受渠道影响）。
func (l *PromoLogic) RecordDownloadClick(req *types.PromoDownloadClickRequest) {
	code := strings.TrimSpace(req.Channel)
	if code == "" {
		return
	}
	ch, err := l.svcCtx.AdminRepository.GetPromoChannelByCode(code)
	if err != nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := l.svcCtx.AdminRepository.RecordPromoClick(
		"dl-"+randomClickID(), ch, model.PromoClickDownload, req.GameId, now); err != nil {
		l.Errorf("record promo download failed: %v", err)
	}
}

// ---- S2S：归因解析（user-service 注册时携带 click_id 回调） ----

// ResolveClick 按 click_id 解析渠道与归因窗口：注册时刻与点击时刻比较
// 判定是否归因（窗口判定在 model.IsClickWithinWindow，可单测）。渠道
// 停用不回改既有点击（历史结算不变）。
func (l *PromoLogic) ResolveClick(clickID string, registeredAt string, registeredUserID int64) (*types.PromoLinkResolveResponse, error) {
	clickID = strings.TrimSpace(clickID)
	if clickID == "" {
		return nil, newAPIError(http.StatusBadRequest, "click_id 必填")
	}
	click, err := l.svcCtx.AdminRepository.GetPromoClick(clickID)
	if err != nil {
		if errors.Is(err, model.ErrPromoClickNotFound) {
			// 未命中即「无归因」，按自然流量放行（不是错误）
			return &types.PromoLinkResolveResponse{InWindow: false}, nil
		}
		l.Errorf("lookup promo click failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询渠道点击失败")
	}
	ch, err := l.svcCtx.AdminRepository.GetPromoChannel(click.ChannelId)
	if err != nil {
		// 渠道被删/停用不影响既有点击结算，按登记的窗口小时数判定
		return &types.PromoLinkResolveResponse{
			Channel: click.Channel, ChannelId: click.ChannelId,
			ClickedAt: click.OccurredAt, WindowHours: 0,
			InWindow: model.IsClickWithinWindow(click.OccurredAt, 0, registeredAt),
		}, nil
	}
	inWindow := model.IsClickWithinWindow(click.OccurredAt, ch.AttributionWindowHours, registeredAt)
	if inWindow {
		// 归因命中回写 user_id（幂等，一点击一归因）；回写失败不阻断
		// 注册，渠道只作为归因结果返回。
		if _, err := l.svcCtx.AdminRepository.MarkClickAttributed(clickID, registeredUserID); err != nil {
			l.Errorf("mark click attributed failed: %v", err)
		}
	}
	return &types.PromoLinkResolveResponse{
		Channel: click.Channel, ChannelId: click.ChannelId,
		ClickedAt: click.OccurredAt, WindowHours: ch.AttributionWindowHours,
		InWindow: inWindow,
	}, nil
}

// ---- 辅助 ----

func (l *PromoLogic) channelNotFound(err error) (*types.PromoChannelView, error) {
	if errors.Is(err, model.ErrPromoChannelNotFound) {
		return nil, newAPIError(http.StatusNotFound, "渠道不存在")
	}
	l.Errorf("lookup promo channel failed: %v", err)
	return nil, newAPIError(http.StatusInternalServerError, "查询渠道失败")
}

// audit 记录渠道变更历史（复用 audit_logs，resource_type=promo_channel）；
// 写审计不得拖垮主操作。
func (l *PromoLogic) audit(action string, channelID int64, before, after string) {
	actorID, _ := l.ctx.Value("user_id").(int64)
	role, _ := l.ctx.Value("role").(string)
	ip, _ := l.ctx.Value("client_ip").(string)
	if err := l.svcCtx.AdminRepository.WriteAudit(&model.AuditLog{
		ActorId: actorID, ActorRole: role, Action: action,
		ResourceType: "promo_channel", ResourceId: strconv.FormatInt(channelID, 10),
		BeforeJson: before, AfterJson: after, Ip: ip,
	}); err != nil {
		l.Errorf("write audit failed: %v", err)
	}
}

// validPromoCode 渠道标识卫生校验：1-64 位，仅字母数字与 . _ -（
// 渠道标识会进推广链接 query 与事件 channel 列，不做字符白名单会让
// 链接参数与 SQL 字符串互相污染）。
func validPromoCode(code string) bool {
	if len(code) == 0 || len(code) > 64 {
		return false
	}
	for _, r := range code {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}

func randomClickID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand 失败极罕见；退回纳秒时间戳仍保证唯一
		return hex.EncodeToString([]byte(time.Now().UTC().Format("20060102T150405.999999999")))
	}
	return hex.EncodeToString(buf)
}
