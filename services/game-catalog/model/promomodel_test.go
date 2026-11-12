package model

import (
	"testing"
)

// GP-403 单层渠道推广（F22）模型单测：渠道登记唯一性、点击幂等、
// 一点击一归因、归因窗口判定、渠道效果计数。

func TestPromoChannel_CreateListUpdate(t *testing.T) {
	m := newTestAdminModel(t)

	ch, err := m.CreatePromoChannel("kol-li", "主播李", 72)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if ch.Id <= 0 || ch.Status != PromoChannelActive || ch.AttributionWindowHours != 72 {
		t.Fatalf("created = %+v", ch)
	}

	// code 全局唯一
	if _, err := m.CreatePromoChannel("kol-li", "重复", 24); err == nil {
		t.Fatal("duplicate code must fail")
	}

	if got, err := m.GetPromoChannelByCode("kol-li"); err != nil || got.Code != "kol-li" || got.Name != "主播李" {
		t.Fatalf("by code = %+v/%v", got, err)
	}
	if _, err := m.GetPromoChannelByCode("nope"); err != ErrPromoChannelNotFound {
		t.Fatalf("missing code err = %v, want ErrPromoChannelNotFound", err)
	}

	m.CreatePromoChannel("tvc", "电视广告", 48)
	list, err := m.ListPromoChannels(10, 0)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %d/%v, want 2", len(list), err)
	}
	if page, err := m.ListPromoChannels(1, 0); err != nil || len(page) != 1 {
		t.Fatalf("page = %d/%v, want 1", len(page), err)
	}

	// 改名与改归因窗口
	upd, err := m.UpdatePromoChannel(ch.Id, "主播李（新）", 120)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if upd.Name != "主播李（新）" || upd.AttributionWindowHours != 120 {
		t.Fatalf("updated = %+v", upd)
	}

	dis, err := m.SetPromoChannelStatus(ch.Id, PromoChannelDisabled)
	if err != nil || dis.Status != PromoChannelDisabled {
		t.Fatalf("status = %+v/%v", dis, err)
	}
}

func TestPromoClick_RecordIdempotentAndAttribution(t *testing.T) {
	m := newTestAdminModel(t)
	ch, err := m.CreatePromoChannel("kol-li", "主播李", 72)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	at := "2026-11-12T10:00:00Z"
	rec, err := m.RecordPromoClick("c-1", ch, PromoClickClick, "g-1", at)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if rec.Id <= 0 || rec.Channel != "kol-li" || rec.UserId != 0 {
		t.Fatalf("recorded = %+v", rec)
	}

	// click_id 唯一键幂等：重放返回同一条，不新增
	again, err := m.RecordPromoClick("c-1", ch, PromoClickClick, "g-1", at)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if again.Id != rec.Id {
		t.Fatalf("replay id = %d, want %d", again.Id, rec.Id)
	}

	// 归因命中回写 user_id
	ok, err := m.MarkClickAttributed("c-1", 42)
	if err != nil || !ok {
		t.Fatalf("attribute = %v/%v", ok, err)
	}
	// 同一用户重复归因：幂等 true，不重复写
	if ok, err := m.MarkClickAttributed("c-1", 42); err != nil || !ok {
		t.Fatalf("attribute same user = %v/%v", ok, err)
	}
	// 渠道间不抢注：他人归因被拒
	if ok, err := m.MarkClickAttributed("c-1", 43); err != nil || ok {
		t.Fatalf("steal = %v/%v, want false/nil", ok, err)
	}

	if rec, err := m.GetPromoClick("c-1"); err != nil || rec.UserId != 42 {
		t.Fatalf("after attribute = %+v/%v", rec, err)
	}
	if _, err := m.GetPromoClick("c-none"); err != ErrPromoClickNotFound {
		t.Fatalf("missing click err = %v, want ErrPromoClickNotFound", err)
	}
}

func TestPromoChannelStats(t *testing.T) {
	m := newTestAdminModel(t)
	ch, err := m.CreatePromoChannel("kol-li", "主播李", 72)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// 窗口外的下载（同渠道）
	if _, err := m.RecordPromoClick("d-0", ch, PromoClickDownload, "g-1", "2026-11-01T00:00:00Z"); err != nil {
		t.Fatalf("record old download: %v", err)
	}
	if _, err := m.RecordPromoClick("d-1", ch, PromoClickDownload, "g-1", "2026-11-12T10:00:00Z"); err != nil {
		t.Fatalf("record download: %v", err)
	}
	if _, err := m.RecordPromoClick("c-1", ch, PromoClickClick, "g-1", "2026-11-12T09:00:00Z"); err != nil {
		t.Fatalf("record click: %v", err)
	}
	// 已归因点击 + 未归因点击各一
	if _, err := m.RecordPromoClick("c-2", ch, PromoClickClick, "g-1", "2026-11-12T09:30:00Z"); err != nil {
		t.Fatalf("record click2: %v", err)
	}
	if _, err := m.MarkClickAttributed("c-1", 42); err != nil {
		t.Fatalf("attribute: %v", err)
	}

	from, to := "2026-11-12T00:00:00Z", "2026-11-12T23:59:59Z"
	s, err := m.ChannelStats("kol-li", from, to)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if s.Clicks != 2 || s.Downloads != 1 || s.Attributes != 1 {
		t.Fatalf("stats = %+v, want clicks=2 downloads=1 attributed=1", s)
	}

	// 全区间（含窗口外下载）
	all, err := m.ChannelStats("kol-li", "", "")
	if err != nil {
		t.Fatalf("stats all: %v", err)
	}
	if all.Clicks != 2 || all.Downloads != 2 || all.Attributes != 1 {
		t.Fatalf("stats all = %+v, want clicks=2 downloads=2 attributed=1", all)
	}

	// 未知渠道：零值不报错
	none, err := m.ChannelStats("ghost", "", "")
	if err != nil {
		t.Fatalf("stats ghost: %v", err)
	}
	if none.Clicks != 0 || none.Downloads != 0 || none.Attributes != 0 {
		t.Fatalf("stats ghost = %+v, want zeros", none)
	}
}

func TestIsClickWithinWindow(t *testing.T) {
	clicked := "2026-11-12T00:00:00Z"
	cases := []struct {
		name  string
		at    string
		hours int64
		want  bool
	}{
		{"同刻", clicked, 72, true},
		{"窗口内", "2026-11-13T00:00:00Z", 72, true},
		{"小时溢出非法", "2026-11-12T50:00:00Z", 72, false},
		{"恰在窗口边界", "2026-11-15T00:00:00Z", 72, true},
		{"刚出窗口", "2026-11-15T00:00:01Z", 72, false},
		{"远超窗口", "2026-11-30T00:00:00Z", 72, false},
		{"长窗口内", "2026-11-20T00:00:00Z", 720, true},
		{"零窗口外", "2026-11-12T00:00:01Z", 0, false},
		{"零窗口同刻", clicked, 0, true},
		{"空发生时刻视为刚刚", "", 72, true},
		{"点击时刻非法", "junk", 72, false},
		{"发生时刻非法", "not-a-time", 72, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsClickWithinWindow(clicked, tc.hours, tc.at); got != tc.want {
				t.Fatalf("IsClickWithinWindow(%q, %d, %q) = %v, want %v",
					clicked, tc.hours, tc.at, got, tc.want)
			}
			// 非法时刻的调用点：点击时刻本身非法
			if got := IsClickWithinWindow("bad-time", 72, tc.at); got != false {
				t.Fatalf("bad click time = %v, want false", got)
			}
		})
	}
}
