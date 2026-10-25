package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func contextWithMember(ctx context.Context, m *Member) context.Context {
	return context.WithValue(ctx, ctxMember, m)
}

// ---- 健康检查 ----

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// ---- 登录 ----

func handleState(w http.ResponseWriter, r *http.Request) {
	ms, err := store.ListMembers()
	if err != nil {
		writeErr(w, 500, "读取成员失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "members": ms})
}

// handleLogin 校验家庭口令。通过后再由前端让用户选身份。
func handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !failLimiterAllow(ip) {
		writeErr(w, 429, "尝试过于频繁，请稍后再试")
		return
	}
	var req struct {
		Passcode string `json:"passcode"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if !secureEqual(strings.TrimSpace(req.Passcode), sharedTok) {
		failLimiterFail(ip)
		writeErr(w, 401, "口令不对")
		return
	}
	failLimiterReset(ip)
	ms, err := store.ListMembers()
	if err != nil {
		writeErr(w, 500, "读取成员失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "members": ms})
}

// handleSession 选定身份后签发长期 token。
func handleSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Passcode string `json:"passcode"`
		MemberID int64  `json:"member_id"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if !secureEqual(strings.TrimSpace(req.Passcode), sharedTok) {
		writeErr(w, 401, "口令不对")
		return
	}
	var exists int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM member WHERE id=? AND is_active=1`, req.MemberID).Scan(&exists); err != nil || exists == 0 {
		writeErr(w, 400, "身份不存在")
		return
	}
	tk, err := store.CreateToken(req.MemberID, 90)
	if err != nil {
		writeErr(w, 500, "签发失败")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "fl_token",
		Value:    tk,
		Path:     "/",
		MaxAge:   90 * 24 * 3600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, map[string]any{"ok": true, "token": tk, "member_id": req.MemberID})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "fl_token", Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, map[string]any{"ok": true})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeErr(w, 400, "请求格式不对")
		return err
	}
	return nil
}

func failLimiterAllow(ip string) bool { return failTrack.allow(ip) }
func failLimiterFail(ip string)       { failTrack.fail(ip) }
func failLimiterReset(ip string)      { failTrack.reset(ip) }

// ---- 成员 ----

func handleListMembers(w http.ResponseWriter, r *http.Request) {
	ms, err := store.ListMembers()
	if err != nil {
		writeErr(w, 500, "读取失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "members": ms})
}

func handleAddMember(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Nickname string `json:"nickname"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	n := strings.TrimSpace(req.Nickname)
	if n == "" {
		writeErr(w, 400, "请填写称呼")
		return
	}
	id, err := store.AddMember(n)
	if err != nil {
		writeErr(w, 400, "该称呼已存在")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

// ---- 往来对象 ----

func handleSearchPersons(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := atoiDefault(r.URL.Query().Get("limit"), 50)
	ps, err := store.SearchPersons(q, limit)
	if err != nil {
		writeErr(w, 500, "查询失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "persons": orEmpty(ps)})
}

func handleCreatePerson(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Relation string `json:"relation"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	id, err := store.EnsurePerson(req.Name, req.Relation, "")
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	p, _ := store.GetPerson(id)
	writeJSON(w, map[string]any{"ok": true, "person": p})
}

func handleGetPerson(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	p, err := store.GetPerson(id)
	if err != nil || p == nil {
		writeErr(w, 404, "找不到这个人")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "person": p})
}

func handleUpdatePerson(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	var req struct {
		Name     string `json:"name"`
		Relation string `json:"relation"`
		Note     string `json:"note"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if err := store.UpdatePerson(id, req.Name, req.Relation, req.Note); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleDeletePerson(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	if err := store.DeletePerson(id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handlePersonRecords(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	list, err := store.ListGifts(id, 200)
	if err != nil {
		writeErr(w, 500, "查询失败")
		return
	}
	p, _ := store.GetPerson(id)
	writeJSON(w, map[string]any{"ok": true, "records": orEmpty(list), "person": p})
}

// ---- 人情 ----

func handleAddGift(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PersonID   int64  `json:"person_id"`
		PersonName string `json:"person_name"`
		Relation   string `json:"relation"`
		EventID    int64  `json:"event_id"`
		EventName  string `json:"event_name"`
		Direction  string `json:"direction"`
		Amount     string `json:"amount"`
		Date       string `json:"date"`
		Method     string `json:"method"`
		Note       string `json:"note"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	pid := req.PersonID
	if pid <= 0 {
		if strings.TrimSpace(req.PersonName) == "" {
			writeErr(w, 400, "请选择或填写姓名")
			return
		}
		var err error
		pid, err = store.EnsurePerson(req.PersonName, req.Relation, "")
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	}
	eid := req.EventID
	if eid <= 0 && strings.TrimSpace(req.EventName) != "" {
		if v, err := store.EnsureEvent(req.EventName); err == nil {
			eid = v
		}
	}
	var by int64
	if m := currentMember(r); m != nil {
		by = m.ID
	}
	id, err := store.AddGift(pid, eid, req.Direction, req.Amount, req.Date, req.Method, req.Note, by)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	p, _ := store.GetPerson(pid)
	sum, _ := store.GiftSummary()
	store.RecordAudit("create", "gift", id, by, req.PersonName+" "+req.Amount)
	writeJSON(w, map[string]any{"ok": true, "id": id, "person": p, "summary": sum})
}

func handleDeleteGift(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	if err := store.DeleteGift(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	store.RecordAudit("delete", "gift", id, memberID(r), "删除人情记录")
	writeJSON(w, map[string]any{"ok": true})
}

func handleListGifts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	personID, _ := strconv.ParseInt(q.Get("person_id"), 10, 64)
	eventID, _ := strconv.ParseInt(q.Get("event_id"), 10, 64)
	minC, _ := strconv.ParseInt(q.Get("min"), 10, 64)
	maxC, _ := strconv.ParseInt(q.Get("max"), 10, 64)
	f := GiftFilter{
		From: q.Get("from"), To: q.Get("to"), PersonID: personID, EventID: eventID,
		Relation: q.Get("relation"), Dir: q.Get("dir"), Min: minC, Max: maxC,
		Keyword: q.Get("q"), Limit: atoiDefault(q.Get("limit"), 200),
	}
	list, err := store.FilterGifts(f)
	if err != nil {
		writeErr(w, 500, "查询失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "records": orEmpty(list)})
}

// handleUpdateGift 改记录：金额、方向、日期、事由、备注、回礼状态。
func handleUpdateGift(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	var req struct {
		Direction string `json:"direction"`
		Amount    string `json:"amount"`
		PersonID  int64  `json:"person_id"`
		EventID   int64  `json:"event_id"`
		Date      string `json:"date"`
		Method    string `json:"method"`
		Note      string `json:"note"`
		Settle    string `json:"settle"`
		Thanks    string `json:"thanks_status"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if err := store.UpdateGift(id, GiftPatch{
		Direction: req.Direction, Amount: req.Amount, PersonID: req.PersonID,
		EventID: req.EventID, Date: req.Date, Method: req.Method, Note: req.Note, Settle: req.Settle, Thanks: req.Thanks,
	}); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	store.RecordAudit("update", "gift", id, memberID(r), "更新人情记录")
	writeJSON(w, map[string]any{"ok": true})
}

func handleUpdateBaby(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	var req struct {
		BabyID   int64   `json:"baby_id"`
		Category string  `json:"category"`
		Amount   string  `json:"amount"`
		Date     string  `json:"date"`
		Merchant string  `json:"merchant"`
		Note     string  `json:"note"`
		Qty      float64 `json:"qty"`
		Unit     string  `json:"unit"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if err := store.UpdateBaby(id, BabyPatch{
		BabyID: req.BabyID, Category: req.Category, Amount: req.Amount, Date: req.Date,
		Merchant: req.Merchant, Note: req.Note, Qty: req.Qty, Unit: req.Unit,
	}); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	store.RecordAudit("update", "baby", id, memberID(r), "更新宝宝开销")
	writeJSON(w, map[string]any{"ok": true})
}

// handleBatchGifts 办酒席批量录礼单。
func handleBatchGifts(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventID   int64       `json:"event_id"`
		EventName string      `json:"event_name"`
		Date      string      `json:"date"`
		Method    string      `json:"method"`
		Items     []BatchItem `json:"items"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	var by int64
	if m := currentMember(r); m != nil {
		by = m.ID
	}
	n, err := store.BatchGifts(req.EventID, req.EventName, req.Date, req.Method, req.Items, by)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "count": n})
}

// handleStats 区间统计：本周/本月/本季/本年/全部，含环比。
func handleStats(w http.ResponseWriter, r *http.Request) {
	rg := r.URL.Query().Get("range")
	if rg == "" {
		rg = "month"
	}
	v, err := store.StatsFor(rg, parseIDDefault(r.URL.Query().Get("baby_id"), 0))
	if err != nil {
		writeErr(w, 500, "统计失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "stats": v})
}

// handleBackup 导出完整 JSON 快照。
func handleBackup(w http.ResponseWriter, r *http.Request) {
	snap, err := store.ExportAll()
	if err != nil {
		writeErr(w, 500, "导出失败")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="familyledger-backup.json"`)
	_ = json.NewEncoder(w).Encode(snap)
}

// handleRestore 从 JSON 快照恢复。
func handleRestore(w http.ResponseWriter, r *http.Request) {
	var snap Snapshot
	if err := decodeJSON(w, r, &snap); err != nil {
		return
	}
	n, err := store.ImportAll(&snap)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "count": n})
}

func handleGiftSummary(w http.ResponseWriter, r *http.Request) {
	sum, err := store.GiftSummary()
	if err != nil {
		writeErr(w, 500, "统计失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "summary": sum})
}

func handleOwes(w http.ResponseWriter, r *http.Request) {
	limit := atoiDefault(r.URL.Query().Get("limit"), 100)
	list, err := store.OweList(limit)
	if err != nil {
		writeErr(w, 500, "统计失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "persons": orEmpty(list)})
}

// ---- 事由 ----

func handleListEvents(w http.ResponseWriter, r *http.Request) {
	list, err := store.ListEvents()
	if err != nil {
		writeErr(w, 500, "查询失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "events": orEmpty(list)})
}

func handleCreateEvent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		Date       string `json:"date"`
		Note       string `json:"note"`
		Status     string `json:"status"`
		Venue      string `json:"venue"`
		Budget     string `json:"budget"`
		GuestCount int    `json:"guest_count"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	id, err := store.CreateEvent(req.Name, req.Date, req.Note, req.Status, req.Venue, req.Budget, req.GuestCount)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	store.RecordAudit("create", "event", id, memberID(r), req.Name)
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

// ---- 添丁记 ----

func handleAddBaby(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BabyID   int64   `json:"baby_id"`
		Category string  `json:"category"`
		Amount   string  `json:"amount"`
		Date     string  `json:"date"`
		Merchant string  `json:"merchant"`
		Note     string  `json:"note"`
		Qty      float64 `json:"qty"`
		Unit     string  `json:"unit"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	var by int64
	if m := currentMember(r); m != nil {
		by = m.ID
	}
	unit := strings.TrimSpace(req.Unit)
	if unit == "" {
		unit = defaultUnit(req.Category)
	}
	id, err := store.AddBabyFor(req.BabyID, req.Category, req.Amount, req.Date, req.Merchant, req.Note, req.Qty, unit, by)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	store.RecordAudit("create", "baby", id, by, req.Category+" "+req.Amount)
	sum, _ := store.BabySummary(6)
	writeJSON(w, map[string]any{"ok": true, "id": id, "summary": sum})
}

func handleDeleteBaby(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	if err := store.DeleteBaby(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	store.RecordAudit("delete", "baby", id, memberID(r), "删除宝宝开销")
	writeJSON(w, map[string]any{"ok": true})
}

func handleListBaby(w http.ResponseWriter, r *http.Request) {
	limit := atoiDefault(r.URL.Query().Get("limit"), 200)
	list, err := store.ListBabyFor(parseIDDefault(r.URL.Query().Get("baby_id"), 0), limit)
	if err != nil {
		writeErr(w, 500, "查询失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "records": orEmpty(list)})
}

func handleBabySummary(w http.ResponseWriter, r *http.Request) {
	months := atoiDefault(r.URL.Query().Get("months"), 6)
	sum, err := store.BabySummaryFor(parseIDDefault(r.URL.Query().Get("baby_id"), 0), months)
	if err != nil {
		writeErr(w, 500, "统计失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "summary": sum})
}

// ---- 洞察 ----

func handleInsights(w http.ResponseWriter, r *http.Request) {
	v, err := store.Insights()
	if err != nil {
		writeErr(w, 500, "计算洞察失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "insights": v})
}

func handleTimeline(w http.ResponseWriter, r *http.Request) {
	limit := atoiDefault(r.URL.Query().Get("limit"), 120)
	list, err := store.Timeline(limit)
	if err != nil {
		writeErr(w, 500, "读取时间线失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "items": orEmpty(list)})
}

func handleConsumables(w http.ResponseWriter, r *http.Request) {
	list, err := store.Consumables()
	if err != nil {
		writeErr(w, 500, "计算失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "consumables": orEmpty(list)})
}

// ---- 成长里程碑 ----

func handleListMilestones(w http.ResponseWriter, r *http.Request) {
	list, err := store.ListMilestonesFor(parseIDDefault(r.URL.Query().Get("baby_id"), 0), 200)
	if err != nil {
		writeErr(w, 500, "读取失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "milestones": orEmpty(list)})
}

func handleAddMilestone(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BabyID int64  `json:"baby_id"`
		Kind   string `json:"kind"`
		Title  string `json:"title"`
		Date   string `json:"date"`
		Note   string `json:"note"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if strings.TrimSpace(req.Kind) == "" && strings.TrimSpace(req.Title) == "" {
		writeErr(w, 400, "请填写内容")
		return
	}
	var by int64
	if m := currentMember(r); m != nil {
		by = m.ID
	}
	id, err := store.AddMilestoneFor(req.BabyID, req.Kind, req.Title, req.Date, req.Note, by)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func handleDeleteMilestone(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	if err := store.DeleteMilestone(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleMilestoneKinds(w http.ResponseWriter, r *http.Request) {
	out := make([]map[string]string, 0, len(defaultMilestones))
	for _, m := range defaultMilestones {
		out = append(out, map[string]string{"kind": m.kind, "title": m.title})
	}
	writeJSON(w, map[string]any{"ok": true, "kinds": out})
}

// ---- 档案设置（宝宝昵称 / 生日） ----

func handleGetProfile(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"ok": true,
		"profile": map[string]string{
			"baby_name":     store.GetProfile("baby_name"),
			"baby_birthday": store.GetProfile("baby_birthday"),
		},
	})
}

func handleSetProfile(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	for k, v := range req {
		if k != "baby_name" && k != "baby_birthday" {
			continue
		}
		if err := store.SetProfile(k, v); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	// 同步到宝宝档案：设置页改的是 profile，多宝宝页读的是 baby_profile，
	// 不同步的话会出现「设置里叫小汤圆、宝宝档案里还是宝宝」。
	if err := store.SyncBabyProfileFromProfile(req["baby_name"], req["baby_birthday"]); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// ---- 导出 ----

func handleExportCSV(w http.ResponseWriter, r *http.Request) {
	book := r.URL.Query().Get("book")
	if book == "" {
		book = "gift"
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	name := "人情账本-" + time.Now().Format("2006-01-02") + ".csv"
	if book == "baby" {
		name = "添丁记-" + time.Now().Format("2006-01-02") + ".csv"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, name, url.QueryEscape(name)))

	cw := csv.NewWriter(w)
	// UTF-8 BOM，保证 Excel 打开中文不乱码
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	defer cw.Flush()

	if book == "baby" {
		_ = cw.Write([]string{"日期", "分类", "金额(元)", "数量", "单位", "商家", "备注", "记录人"})
		list, err := store.ListBabyFor(parseIDDefault(r.URL.Query().Get("baby_id"), 0), 100000)
		if err != nil {
			writeErr(w, 500, "导出失败")
			return
		}
		for _, b := range list {
			_ = cw.Write([]string{b.On, b.Category, centsToYuan(b.Cents), strFloat(b.Qty), b.Unit, b.Merchant, b.Note, b.CreatedBy})
		}
		return
	}
	_ = cw.Write([]string{"日期", "姓名", "关系", "方向", "金额(元)", "事由", "方式", "备注", "记录人"})
	list, err := store.ListGifts(0, 100000)
	if err != nil {
		writeErr(w, 500, "导出失败")
		return
	}
	for _, g := range list {
		dir := "收礼"
		if g.Direction == "out" {
			dir = "送礼"
		}
		_ = cw.Write([]string{g.On, g.Person, "", dir, centsToYuan(g.Cents), g.Event, g.Method, g.Note, g.CreatedBy})
	}
}

func centsToYuan(c int64) string {
	return fmt.Sprintf("%d.%02d", c/100, abs64(c%100))
}

func strFloat(v float64) string {
	if v == 0 {
		return ""
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// ---- 工具 ----

func pathID(r *http.Request, key string) (int64, error) {
	v := r.PathValue(key)
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("bad id")
	}
	return id, nil
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func orEmpty[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}
