package logic

import (
	"errors"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"
)

// failingPromoStore GP-403 渠道推广 logic 错误分支注入：内嵌真实仓储，仅按
// 字段覆写指定方法；未覆写方法委托真实仓储。
type failingPromoStore struct {
	model.AdminStore
	createChannelErr    error
	getChannelErr       error
	getChannelByCodeErr error
	listChannelsErr     error
	updateChannelErr    error
	setStatusErr        error
	recordClickErr      error
	getClickErr         error
	markAttributedErr   error
	channelStatsErr     error
	writeAuditErr       error
}

var _ model.AdminStore = (*failingPromoStore)(nil)

func (f *failingPromoStore) CreatePromoChannel(code, name string, windowHours int64) (*model.PromoChannel, error) {
	if f.createChannelErr != nil {
		return nil, f.createChannelErr
	}
	return f.AdminStore.CreatePromoChannel(code, name, windowHours)
}

func (f *failingPromoStore) GetPromoChannel(id int64) (*model.PromoChannel, error) {
	if f.getChannelErr != nil {
		return nil, f.getChannelErr
	}
	return f.AdminStore.GetPromoChannel(id)
}

func (f *failingPromoStore) GetPromoChannelByCode(code string) (*model.PromoChannel, error) {
	if f.getChannelByCodeErr != nil {
		return nil, f.getChannelByCodeErr
	}
	return f.AdminStore.GetPromoChannelByCode(code)
}

func (f *failingPromoStore) ListPromoChannels(limit, offset int) ([]*model.PromoChannel, error) {
	if f.listChannelsErr != nil {
		return nil, f.listChannelsErr
	}
	return f.AdminStore.ListPromoChannels(limit, offset)
}

func (f *failingPromoStore) UpdatePromoChannel(id int64, name string, windowHours int64) (*model.PromoChannel, error) {
	if f.updateChannelErr != nil {
		return nil, f.updateChannelErr
	}
	return f.AdminStore.UpdatePromoChannel(id, name, windowHours)
}

func (f *failingPromoStore) SetPromoChannelStatus(id int64, status string) (*model.PromoChannel, error) {
	if f.setStatusErr != nil {
		return nil, f.setStatusErr
	}
	return f.AdminStore.SetPromoChannelStatus(id, status)
}

func (f *failingPromoStore) RecordPromoClick(clickId string, ch *model.PromoChannel, kind, gameId, now string) (*model.PromoClick, error) {
	if f.recordClickErr != nil {
		return nil, f.recordClickErr
	}
	return f.AdminStore.RecordPromoClick(clickId, ch, kind, gameId, now)
}

func (f *failingPromoStore) GetPromoClick(clickId string) (*model.PromoClick, error) {
	if f.getClickErr != nil {
		return nil, f.getClickErr
	}
	return f.AdminStore.GetPromoClick(clickId)
}

func (f *failingPromoStore) MarkClickAttributed(clickId string, userId int64) (bool, error) {
	if f.markAttributedErr != nil {
		return false, f.markAttributedErr
	}
	return f.AdminStore.MarkClickAttributed(clickId, userId)
}

func (f *failingPromoStore) ChannelStats(channelCode, from, to string) (*model.ChannelStats, error) {
	if f.channelStatsErr != nil {
		return nil, f.channelStatsErr
	}
	return f.AdminStore.ChannelStats(channelCode, from, to)
}

func (f *failingPromoStore) WriteAudit(l *model.AuditLog) error {
	if f.writeAuditErr != nil {
		return f.writeAuditErr
	}
	return f.AdminStore.WriteAudit(l)
}

// newPromoCtx 渠道推广测试上下文：真实 SQLite 仓储 + 一条 active 渠道
// "kol-li"（72h 窗口）；返回 svcCtx 与渠道 id。
func newPromoCtx(t *testing.T) (*svc.ServiceContext, int64) {
	t.Helper()
	svcCtx := newLogicSvcCtx(t)
	ch, err := svcCtx.AdminRepository.CreatePromoChannel("kol-li", "KOL 李", 72)
	if err != nil {
		t.Fatalf("CreatePromoChannel: %v", err)
	}
	return svcCtx, ch.Id
}

// 登记渠道：非法 code/名/窗口 → 400；唯一键冲突 → 409；落库失败 → 500。
func TestPromoLogic_CreateChannelErrors(t *testing.T) {
	t.Run("bad code 400", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		l := NewPromoLogic(adminCtx(1, "operator"), svcCtx)
		_, err := l.CreatePromoChannel(&types.CreatePromoChannelRequest{Code: "含中文", Name: "x"})
		requireAPIStatus(t, err, 400)
	})
	t.Run("empty name 400", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		l := NewPromoLogic(adminCtx(1, "operator"), svcCtx)
		_, err := l.CreatePromoChannel(&types.CreatePromoChannelRequest{Code: "abc", Name: " "})
		requireAPIStatus(t, err, 400)
	})
	t.Run("window out of range 400", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		l := NewPromoLogic(adminCtx(1, "operator"), svcCtx)
		_, err := l.CreatePromoChannel(&types.CreatePromoChannelRequest{Code: "abc", Name: "x", WindowHours: 99999})
		requireAPIStatus(t, err, 400)
	})
	t.Run("duplicate 409", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		l := NewPromoLogic(adminCtx(1, "operator"), svcCtx)
		_, err := l.CreatePromoChannel(&types.CreatePromoChannelRequest{Code: "kol-li", Name: "dup"})
		requireAPIStatus(t, err, 409)
	})
	t.Run("create failed 500", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		svcCtx.AdminRepository = &failingPromoStore{
			AdminStore:       svcCtx.AdminRepository,
			createChannelErr: errors.New("create down"),
		}
		l := NewPromoLogic(adminCtx(1, "operator"), svcCtx)
		_, err := l.CreatePromoChannel(&types.CreatePromoChannelRequest{Code: "abc", Name: "x"})
		requireAPIStatus(t, err, 500)
	})
}

// 变更渠道：查不到 → 404；查询失败 → 500；超长名 → 400；窗口越界 → 400；
// 写入失败 → 500。
func TestPromoLogic_UpdateChannelErrors(t *testing.T) {
	t.Run("not found 404", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		svcCtx.AdminRepository = &failingPromoStore{
			AdminStore:    svcCtx.AdminRepository,
			getChannelErr: model.ErrPromoChannelNotFound,
		}
		l := NewPromoLogic(adminCtx(1, "operator"), svcCtx)
		_, err := l.UpdatePromoChannel(&types.UpdatePromoChannelRequest{Id: 99, Name: "x"})
		requireAPIStatus(t, err, 404)
	})
	t.Run("query failed 500", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		svcCtx.AdminRepository = &failingPromoStore{
			AdminStore:    svcCtx.AdminRepository,
			getChannelErr: errors.New("query down"),
		}
		l := NewPromoLogic(adminCtx(1, "operator"), svcCtx)
		_, err := l.UpdatePromoChannel(&types.UpdatePromoChannelRequest{Id: 99, Name: "x"})
		requireAPIStatus(t, err, 500)
	})
	t.Run("long name 400", func(t *testing.T) {
		svcCtx, chID := newPromoCtx(t)
		l := NewPromoLogic(adminCtx(1, "operator"), svcCtx)
		_, err := l.UpdatePromoChannel(&types.UpdatePromoChannelRequest{Id: chID, Name: string(make([]byte, 129))})
		requireAPIStatus(t, err, 400)
	})
	t.Run("window out of range 400", func(t *testing.T) {
		svcCtx, chID := newPromoCtx(t)
		l := NewPromoLogic(adminCtx(1, "operator"), svcCtx)
		_, err := l.UpdatePromoChannel(&types.UpdatePromoChannelRequest{Id: chID, WindowHours: 99999})
		requireAPIStatus(t, err, 400)
	})
	t.Run("update failed 500", func(t *testing.T) {
		svcCtx, chID := newPromoCtx(t)
		svcCtx.AdminRepository = &failingPromoStore{
			AdminStore:       svcCtx.AdminRepository,
			updateChannelErr: errors.New("update down"),
		}
		l := NewPromoLogic(adminCtx(1, "operator"), svcCtx)
		_, err := l.UpdatePromoChannel(&types.UpdatePromoChannelRequest{Id: chID, Name: "newname"})
		requireAPIStatus(t, err, 500)
	})
}

// 渠道状态：查不到 → 404；非法状态 → 400；写入失败 → 500。
func TestPromoLogic_SetChannelStatusErrors(t *testing.T) {
	t.Run("not found 404", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		svcCtx.AdminRepository = &failingPromoStore{
			AdminStore:    svcCtx.AdminRepository,
			getChannelErr: model.ErrPromoChannelNotFound,
		}
		l := NewPromoLogic(adminCtx(1, "operator"), svcCtx)
		_, err := l.SetPromoChannelStatus(&types.SetPromoChannelStatusRequest{Id: 99, Status: "active"})
		requireAPIStatus(t, err, 404)
	})
	t.Run("invalid status 400", func(t *testing.T) {
		svcCtx, chID := newPromoCtx(t)
		l := NewPromoLogic(adminCtx(1, "operator"), svcCtx)
		_, err := l.SetPromoChannelStatus(&types.SetPromoChannelStatusRequest{Id: chID, Status: "bogus"})
		requireAPIStatus(t, err, 400)
	})
	t.Run("write failed 500", func(t *testing.T) {
		svcCtx, chID := newPromoCtx(t)
		svcCtx.AdminRepository = &failingPromoStore{
			AdminStore:   svcCtx.AdminRepository,
			setStatusErr: errors.New("status down"),
		}
		l := NewPromoLogic(adminCtx(1, "operator"), svcCtx)
		_, err := l.SetPromoChannelStatus(&types.SetPromoChannelStatusRequest{Id: chID, Status: "disabled"})
		requireAPIStatus(t, err, 500)
	})
}

// 渠道列表查询失败 → 500。
func TestPromoLogic_ListChannelsFailed500(t *testing.T) {
	svcCtx, _ := newPromoCtx(t)
	svcCtx.AdminRepository = &failingPromoStore{
		AdminStore:      svcCtx.AdminRepository,
		listChannelsErr: errors.New("list down"),
	}
	l := NewPromoLogic(adminCtx(1, "operator"), svcCtx)
	_, err := l.ListPromoChannels(0, 0)
	requireAPIStatus(t, err, 500)
}

// 渠道计数：空 code → 400；查询失败 → 500。
func TestPromoLogic_ChannelStatsErrors(t *testing.T) {
	t.Run("empty code 400", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		l := NewPromoLogic(adminCtx(1, "operator"), svcCtx)
		_, err := l.ChannelStats(" ", "", "")
		requireAPIStatus(t, err, 400)
	})
	t.Run("stats failed 500", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		svcCtx.AdminRepository = &failingPromoStore{
			AdminStore:      svcCtx.AdminRepository,
			channelStatsErr: errors.New("stats down"),
		}
		l := NewPromoLogic(adminCtx(1, "operator"), svcCtx)
		_, err := l.ChannelStats("kol-li", "", "")
		requireAPIStatus(t, err, 500)
	})
}

// 落地链接：空 code → 400；渠道不存在/停用 → 404；查询失败 → 500；点击
// 落库失败 → 500；成功自生成 click_id。
func TestPromoLogic_ResolveLinkErrors(t *testing.T) {
	t.Run("empty code 400", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		l := NewPromoLogic(reviewCtx(7), svcCtx)
		_, err := l.ResolvePromoLink(&types.ResolvePromoLinkRequest{Channel: " "})
		requireAPIStatus(t, err, 400)
	})
	t.Run("channel not found 404", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		svcCtx.AdminRepository = &failingPromoStore{
			AdminStore:          svcCtx.AdminRepository,
			getChannelByCodeErr: model.ErrPromoChannelNotFound,
		}
		l := NewPromoLogic(reviewCtx(7), svcCtx)
		_, err := l.ResolvePromoLink(&types.ResolvePromoLinkRequest{Channel: "ghost"})
		requireAPIStatus(t, err, 404)
	})
	t.Run("channel disabled 404", func(t *testing.T) {
		svcCtx, chID := newPromoCtx(t)
		if _, err := svcCtx.AdminRepository.SetPromoChannelStatus(chID, model.PromoChannelDisabled); err != nil {
			t.Fatalf("SetPromoChannelStatus: %v", err)
		}
		l := NewPromoLogic(reviewCtx(7), svcCtx)
		_, err := l.ResolvePromoLink(&types.ResolvePromoLinkRequest{Channel: "kol-li"})
		requireAPIStatus(t, err, 404)
	})
	t.Run("lookup failed 500", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		svcCtx.AdminRepository = &failingPromoStore{
			AdminStore:          svcCtx.AdminRepository,
			getChannelByCodeErr: errors.New("lookup down"),
		}
		l := NewPromoLogic(reviewCtx(7), svcCtx)
		_, err := l.ResolvePromoLink(&types.ResolvePromoLinkRequest{Channel: "kol-li"})
		requireAPIStatus(t, err, 500)
	})
	t.Run("click record failed 500", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		svcCtx.AdminRepository = &failingPromoStore{
			AdminStore:     svcCtx.AdminRepository,
			recordClickErr: errors.New("click down"),
		}
		l := NewPromoLogic(reviewCtx(7), svcCtx)
		_, err := l.ResolvePromoLink(&types.ResolvePromoLinkRequest{Channel: "kol-li"})
		requireAPIStatus(t, err, 500)
	})
	t.Run("success generates click id", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		l := NewPromoLogic(reviewCtx(7), svcCtx)
		resp, err := l.ResolvePromoLink(&types.ResolvePromoLinkRequest{Channel: "kol-li", GameId: "g-rpg-1"})
		if err != nil {
			t.Fatalf("ResolvePromoLink: %v", err)
		}
		if resp.Channel != "kol-li" || resp.ClickId == "" || resp.WindowSecs != 72*3600 {
			t.Fatalf("resp = %+v", resp)
		}
	})
}

// 归因解析：空 click_id → 400；点击不存在 → 自然流量放行；查询失败 → 500；
// 渠道被删（GetPromoChannel 报错）→ 按登记窗口 0 判定；命中回写失败仅记日志。
func TestPromoLogic_ResolveClickErrors(t *testing.T) {
	t.Run("empty click id 400", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		l := NewPromoLogic(reviewCtx(7), svcCtx)
		_, err := l.ResolveClick(" ", "", 7)
		requireAPIStatus(t, err, 400)
	})
	t.Run("click not found natural flow", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		l := NewPromoLogic(reviewCtx(7), svcCtx)
		resp, err := l.ResolveClick("click-ghost", "2026-11-22T00:00:00Z", 7)
		if err != nil || resp.InWindow {
			t.Fatalf("want natural flow: resp=%+v err=%v", resp, err)
		}
	})
	t.Run("lookup failed 500", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		svcCtx.AdminRepository = &failingPromoStore{
			AdminStore:  svcCtx.AdminRepository,
			getClickErr: errors.New("click down"),
		}
		l := NewPromoLogic(reviewCtx(7), svcCtx)
		_, err := l.ResolveClick("click-1", "", 7)
		requireAPIStatus(t, err, 500)
	})
	t.Run("channel gone resolves by recorded window", func(t *testing.T) {
		svcCtx, chID := newPromoCtx(t)
		if _, err := svcCtx.AdminRepository.RecordPromoClick("click-1", &model.PromoChannel{Id: chID, Code: "kol-li", AttributionWindowHours: 72, Status: model.PromoChannelActive}, model.PromoClickClick, "g-rpg-1", "2026-11-20T00:00:00Z"); err != nil {
			t.Fatalf("RecordPromoClick: %v", err)
		}
		svcCtx.AdminRepository = &failingPromoStore{
			AdminStore:    svcCtx.AdminRepository,
			getChannelErr: model.ErrPromoChannelNotFound,
		}
		l := NewPromoLogic(reviewCtx(7), svcCtx)
		resp, err := l.ResolveClick("click-1", "2026-11-22T00:00:00Z", 7)
		if err != nil || resp.Channel != "kol-li" || resp.WindowHours != 0 {
			t.Fatalf("resp=%+v err=%v", resp, err)
		}
	})
	t.Run("mark failed tolerated", func(t *testing.T) {
		svcCtx, chID := newPromoCtx(t)
		if _, err := svcCtx.AdminRepository.RecordPromoClick("click-2", &model.PromoChannel{Id: chID, Code: "kol-li", AttributionWindowHours: 72, Status: model.PromoChannelActive}, model.PromoClickClick, "g-rpg-1", "2026-11-20T00:00:00Z"); err != nil {
			t.Fatalf("RecordPromoClick: %v", err)
		}
		svcCtx.AdminRepository = &failingPromoStore{
			AdminStore:        svcCtx.AdminRepository,
			markAttributedErr: errors.New("mark down"),
		}
		l := NewPromoLogic(reviewCtx(7), svcCtx)
		resp, err := l.ResolveClick("click-2", "2026-11-20T10:00:00Z", 7)
		if err != nil || !resp.InWindow || resp.Channel != "kol-li" {
			t.Fatalf("resp=%+v err=%v", resp, err)
		}
	})
}

// 下载登记：空 channel 直接返回；渠道无效不计（不报错）；落库失败仅记日志。
func TestPromoLogic_RecordDownloadClick(t *testing.T) {
	t.Run("empty channel returns", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		l := NewPromoLogic(reviewCtx(7), svcCtx)
		l.RecordDownloadClick(&types.PromoDownloadClickRequest{Channel: " ", GameId: "g-rpg-1"}) // no panic
	})
	t.Run("invalid channel ignored", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		l := NewPromoLogic(reviewCtx(7), svcCtx)
		l.RecordDownloadClick(&types.PromoDownloadClickRequest{Channel: "ghost", GameId: "g-rpg-1"}) // no panic
	})
	t.Run("record failed logged", func(t *testing.T) {
		svcCtx, _ := newPromoCtx(t)
		svcCtx.AdminRepository = &failingPromoStore{
			AdminStore:     svcCtx.AdminRepository,
			recordClickErr: errors.New("click down"),
		}
		l := NewPromoLogic(reviewCtx(7), svcCtx)
		l.RecordDownloadClick(&types.PromoDownloadClickRequest{Channel: "kol-li", GameId: "g-rpg-1"}) // no panic
	})
}
