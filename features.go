package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// 第二阶段能力的存储层：事件工作台、家庭协作、关系洞察、附件和审计。

type EventPatch struct {
	Name          string `json:"name"`
	Date          string `json:"date"`
	// Note / Venue 用指针：传 "" 表示「清空」，不传（nil）才表示「不改」。
	// 用 string 的话空串和不传无法区分，用户就永远清不掉已经填错的内容。
	Note          *string `json:"note"`
	Status        string  `json:"status"`
	Venue         *string `json:"venue"`
	Budget        string  `json:"budget"`
	GuestCount    int     `json:"guest_count"`
	BudgetSet     bool    `json:"budget_set"`
	GuestCountSet bool    `json:"guest_count_set"`
}

type EventDetail struct {
	Event         Event  `json:"event"`
	Gifts         []Gift `json:"gifts"`
	RecvCents     int64  `json:"recv_cents"`
	SentCents     int64  `json:"sent_cents"`
	PendingCents  int64  `json:"pending_cents"`
	ThanksPending int    `json:"thanks_pending"`
	GuestCount    int    `json:"guest_count"`
}

func (s *Store) CreateEvent(name, date, note, status, venue, budget string, guestCount int) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("事由不能为空")
	}
	// 这里不能复用 EnsureEvent：它会按名字找到已有事件，再把本次填的资料写回去，
	// 等于用新事件静默覆盖旧事件（同一个事由办两次时，上一次的资料就丢了）。
	// gift_event.name 是 UNIQUE，同名事由在库里本就不能共存，所以直接明确报错。
	var dup int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM gift_event WHERE name=?`, name).Scan(&dup); err == nil && dup > 0 {
		return 0, errors.New("已存在同名事由，请换个名字，或打开已有事由修改")
	}
	budgetCents := int64(0)
	if strings.TrimSpace(budget) != "" {
		c, err := ParseMoney(budget)
		if err != nil || c < 0 {
			return 0, errors.New("预算金额不合法")
		}
		budgetCents = c
	}
	if guestCount < 0 {
		guestCount = 0
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = "planned"
	}
	if status != "planned" && status != "active" && status != "done" {
		return 0, errors.New("事件状态不合法")
	}
	res, err := s.db.Exec(`INSERT INTO gift_event(name,event_date,note,status,venue,budget_cents,guest_count) VALUES(?,?,?,?,?,?,?)`,
		name, validDate(date), strings.TrimSpace(note), status, strings.TrimSpace(venue), budgetCents, guestCount)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateEvent(id int64, p EventPatch) error {
	if id <= 0 {
		return errors.New("事件编号不合法")
	}
	sets := []string{}
	args := []any{}
	if strings.TrimSpace(p.Name) != "" {
		sets = append(sets, "name=?")
		args = append(args, strings.TrimSpace(p.Name))
	}
	if p.Date != "" {
		sets = append(sets, "event_date=?")
		args = append(args, validDate(p.Date))
	}
	if p.Note != nil {
		sets = append(sets, "note=?")
		args = append(args, strings.TrimSpace(*p.Note))
	}
	if p.Status != "" {
		if p.Status != "planned" && p.Status != "active" && p.Status != "done" {
			return errors.New("事件状态不合法")
		}
		sets = append(sets, "status=?")
		args = append(args, p.Status)
	}
	if p.Venue != nil {
		sets = append(sets, "venue=?")
		args = append(args, strings.TrimSpace(*p.Venue))
	}
	if p.Budget != "" || p.BudgetSet {
		c := int64(0)
		if strings.TrimSpace(p.Budget) != "" {
			var err error
			c, err = ParseMoney(p.Budget)
			if err != nil || c < 0 {
				return errors.New("预算金额不合法")
			}
		}
		sets = append(sets, "budget_cents=?")
		args = append(args, c)
	}
	if p.GuestCount > 0 || p.GuestCountSet {
		if p.GuestCount < 0 {
			return errors.New("计划人数不能为负数")
		}
		sets = append(sets, "guest_count=?")
		args = append(args, p.GuestCount)
	}
	if len(sets) == 0 {
		return errors.New("没有要改的内容")
	}
	args = append(args, id)
	_, err := s.db.Exec(`UPDATE gift_event SET `+strings.Join(sets, ",")+` WHERE id=?`, args...)
	return err
}

func (s *Store) EventDetail(id int64) (EventDetail, error) {
	var out EventDetail
	var err error
	events, err := s.ListEvents()
	if err != nil {
		return out, err
	}
	for _, e := range events {
		if e.ID == id {
			out.Event = e
			break
		}
	}
	if out.Event.ID == 0 {
		return out, errors.New("找不到这个事件")
	}
	out.Gifts, err = s.FilterGifts(GiftFilter{EventID: id, Limit: 1000000})
	if err != nil {
		return out, err
	}
	for _, g := range out.Gifts {
		if g.Direction == "in" {
			out.RecvCents += g.Cents
			if g.Settle == "" {
				out.PendingCents += g.Cents
			}
		}
		if g.Direction == "out" {
			out.SentCents += g.Cents
		}
		if g.Direction == "in" && g.Thanks == "pending" {
			out.ThanksPending++
		}
	}
	out.GuestCount = out.Event.GuestCount
	return out, nil
}

type FamilyTask struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Note       string `json:"note,omitempty"`
	Kind       string `json:"kind"`
	DueOn      string `json:"due_on,omitempty"`
	AssigneeID int64  `json:"assignee_id,omitempty"`
	Assignee   string `json:"assignee,omitempty"`
	Status     string `json:"status"`
	CreatedBy  string `json:"created_by,omitempty"`
	CreatedAt  string `json:"created_at"`
}

func (s *Store) ListTasks(status string, limit int) ([]FamilyTask, error) {
	if limit <= 0 {
		limit = 200
	}
	where := ""
	args := []any{}
	if status != "" {
		where = "WHERE t.status=?"
		args = append(args, status)
	}
	args = append(args, limit)
	rows, err := s.db.Query(`
		SELECT t.id, t.title, COALESCE(t.note,''), t.kind, COALESCE(t.due_on,''),
		       COALESCE(t.assignee_id,0), COALESCE(a.nickname,''), t.status,
		       COALESCE(c.nickname,''), t.created_at
		FROM family_task t
		LEFT JOIN member a ON a.id=t.assignee_id
		LEFT JOIN member c ON c.id=t.created_by `+where+`
		ORDER BY CASE WHEN t.status='open' THEN 0 ELSE 1 END,
		         CASE WHEN t.due_on='' OR t.due_on IS NULL THEN '9999-12-31' ELSE t.due_on END,
		         t.id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FamilyTask
	for rows.Next() {
		var v FamilyTask
		if err := rows.Scan(&v.ID, &v.Title, &v.Note, &v.Kind, &v.DueOn, &v.AssigneeID, &v.Assignee, &v.Status, &v.CreatedBy, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) AddTask(title, note, kind, due string, assignee, by int64) (int64, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return 0, errors.New("请填写事项")
	}
	if kind == "" {
		kind = "家庭事项"
	}
	var dueArg any
	if strings.TrimSpace(due) != "" {
		dueArg = validDate(due)
	}
	res, err := s.db.Exec(`INSERT INTO family_task(title,note,kind,due_on,assignee_id,created_by) VALUES(?,?,?,?,?,?)`,
		title, strings.TrimSpace(note), kind, dueArg, nullID(assignee), nullID(by))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateTask(id int64, title, note, kind, due string, assignee int64, status string) error {
	sets := []string{}
	args := []any{}
	if strings.TrimSpace(title) != "" {
		sets = append(sets, "title=?")
		args = append(args, strings.TrimSpace(title))
	}
	if note != "" {
		sets = append(sets, "note=?")
		args = append(args, strings.TrimSpace(note))
	}
	if kind != "" {
		sets = append(sets, "kind=?")
		args = append(args, kind)
	}
	if due != "" {
		sets = append(sets, "due_on=?")
		args = append(args, validDate(due))
	}
	if assignee > 0 {
		sets = append(sets, "assignee_id=?")
		args = append(args, assignee)
	}
	if status != "" {
		if status != "open" && status != "done" && status != "archived" {
			return errors.New("事项状态不合法")
		}
		sets = append(sets, "status=?")
		args = append(args, status)
	}
	if len(sets) == 0 {
		return errors.New("没有要改的内容")
	}
	sets = append(sets, "updated_at=datetime('now','localtime')")
	args = append(args, id)
	_, err := s.db.Exec(`UPDATE family_task SET `+strings.Join(sets, ",")+` WHERE id=?`, args...)
	return err
}

func (s *Store) DeleteTask(id int64) error {
	_, err := s.db.Exec(`DELETE FROM family_task WHERE id=?`, id)
	return err
}

type ShoppingItem struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Category   string  `json:"category,omitempty"`
	Qty        float64 `json:"qty,omitempty"`
	Unit       string  `json:"unit,omitempty"`
	Checked    bool    `json:"checked"`
	BoughtBy   string  `json:"bought_by,omitempty"`
	BoughtByID int64   `json:"bought_by_id,omitempty"`
	Note       string  `json:"note,omitempty"`
}

func (s *Store) ListShopping(limit int) ([]ShoppingItem, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.Query(`
		SELECT i.id, i.name, COALESCE(i.category,''), COALESCE(i.qty,0), COALESCE(i.unit,''),
		       i.checked, COALESCE(i.bought_by,0), COALESCE(m.nickname,''), COALESCE(i.note,'')
		FROM shopping_item i LEFT JOIN member m ON m.id=i.bought_by
		ORDER BY i.checked, i.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ShoppingItem
	for rows.Next() {
		var v ShoppingItem
		var checked int
		if err := rows.Scan(&v.ID, &v.Name, &v.Category, &v.Qty, &v.Unit, &checked, &v.BoughtByID, &v.BoughtBy, &v.Note); err != nil {
			return nil, err
		}
		v.Checked = checked != 0
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) AddShopping(name, category string, qty float64, unit, note string, by int64) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("请填写采购内容")
	}
	res, err := s.db.Exec(`INSERT INTO shopping_item(name,category,qty,unit,note,created_by) VALUES(?,?,?,?,?,?)`,
		name, strings.TrimSpace(category), qty, strings.TrimSpace(unit), strings.TrimSpace(note), nullID(by))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateShopping(id int64, name, category string, qty float64, unit, note string, checked bool, boughtBy int64) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("采购内容不能为空")
	}
	_, err := s.db.Exec(`UPDATE shopping_item SET name=?, category=?, qty=?, unit=?, note=?, checked=?, bought_by=?, updated_at=datetime('now','localtime') WHERE id=?`,
		strings.TrimSpace(name), strings.TrimSpace(category), qty, strings.TrimSpace(unit), strings.TrimSpace(note), boolInt(checked), nullID(boughtBy), id)
	return err
}

func (s *Store) DeleteShopping(id int64) error {
	_, err := s.db.Exec(`DELETE FROM shopping_item WHERE id=?`, id)
	return err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

type RelationshipInsight struct {
	PersonID       int64  `json:"person_id"`
	Name           string `json:"name"`
	Relation       string `json:"relation,omitempty"`
	Temperature    string `json:"temperature"`
	DaysSince      int    `json:"days_since"`
	NetCents       int64  `json:"net_cents"`
	SuggestedCents int64  `json:"suggested_cents"`
	PendingThanks  int    `json:"pending_thanks"`
	LastOn         string `json:"last_on,omitempty"`
}

func (s *Store) RelationshipInsights(limit int) ([]RelationshipInsight, error) {
	persons, err := s.SearchPersons("", limit)
	if err != nil {
		return nil, err
	}
	// 原先每人都发两条 SQL，200 人就是 400+ 条，人一多会明显卡。
	// 改成两条聚合查询一次性取回「最后一次收到的礼金」和「待感谢条数」。
	lastIn := map[int64]int64{}
	if rows, err := s.db.Query(`
		SELECT g.person_id, g.amount_cents FROM gift_record g
		WHERE g.direction='in' AND g.id = (
			SELECT g2.id FROM gift_record g2
			WHERE g2.person_id=g.person_id AND g2.direction='in'
			ORDER BY g2.occurred_on DESC, g2.id DESC LIMIT 1
		)`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var pid, cents int64
			if err := rows.Scan(&pid, &cents); err == nil {
				lastIn[pid] = cents
			}
		}
	}
	pendingThanks := map[int64]int64{}
	if rows, err := s.db.Query(`
		SELECT person_id, COUNT(*) FROM gift_record
		WHERE direction='in' AND COALESCE(thanks_status,'pending')='pending'
		GROUP BY person_id`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var pid, n int64
			if err := rows.Scan(&pid, &n); err == nil {
				pendingThanks[pid] = n
			}
		}
	}
	var out []RelationshipInsight
	for _, p := range persons {
		days := daysSince(p.LastOn)
		temperature := "热络"
		switch {
		case days > 730:
			temperature = "沉寂"
		case days > 365:
			temperature = "偏冷"
		case days > 120:
			temperature = "平稳"
		}
		out = append(out, RelationshipInsight{
			PersonID: p.ID, Name: p.Name, Relation: p.Relation, Temperature: temperature,
			DaysSince: days, NetCents: p.NetCents, SuggestedCents: lastIn[p.ID],
			PendingThanks: int(pendingThanks[p.ID]), LastOn: p.LastOn,
		})
	}
	return out, nil
}

type AuditLog struct {
	ID        int64  `json:"id"`
	Action    string `json:"action"`
	Entity    string `json:"entity"`
	EntityID  int64  `json:"entity_id,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Member    string `json:"member,omitempty"`
	CreatedAt string `json:"created_at"`
}

func (s *Store) RecordAudit(action, entity string, entityID, memberID int64, detail string) {
	_, _ = s.db.Exec(`INSERT INTO audit_log(action,entity,entity_id,detail,member_id) VALUES(?,?,?,?,?)`,
		action, entity, nullID(entityID), detail, nullID(memberID))
}

func (s *Store) ListAudit(limit int) ([]AuditLog, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.Query(`
		SELECT a.id,a.action,a.entity,COALESCE(a.entity_id,0),COALESCE(a.detail,''),COALESCE(m.nickname,''),a.created_at
		FROM audit_log a LEFT JOIN member m ON m.id=a.member_id
		ORDER BY a.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditLog
	for rows.Next() {
		var v AuditLog
		if err := rows.Scan(&v.ID, &v.Action, &v.Entity, &v.EntityID, &v.Detail, &v.Member, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type Attachment struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	RecordID  int64  `json:"record_id"`
	FileName  string `json:"file_name"`
	Mime      string `json:"mime"`
	CreatedBy string `json:"created_by,omitempty"`
	CreatedAt string `json:"created_at"`
}

func (s *Store) AddAttachment(kind string, recordID int64, fileName, mime string, data []byte, by int64) (int64, error) {
	if kind == "" || recordID <= 0 || fileName == "" || mime == "" || len(data) == 0 {
		return 0, errors.New("附件信息不完整")
	}
	table := map[string]string{"event": "gift_event", "gift": "gift_record", "baby": "baby_expense", "milestone": "milestone", "health": "health_record", "baby_profile": "baby_profile"}[kind]
	if table == "" {
		return 0, errors.New("附件类型不合法")
	}
	var exists int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE id=?`, recordID).Scan(&exists); err != nil || exists == 0 {
		return 0, errors.New("关联记录不存在")
	}
	mime = http.DetectContentType(data)
	if mime != "image/png" && mime != "image/jpeg" && mime != "image/gif" && mime != "image/webp" && mime != "application/pdf" {
		return 0, errors.New("仅支持图片和 PDF")
	}
	res, err := s.db.Exec(`INSERT INTO attachment(kind,record_id,file_name,mime,data,created_by) VALUES(?,?,?,?,?,?)`,
		kind, recordID, fileName, mime, data, nullID(by))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListAttachments(kind string, recordID int64) ([]Attachment, error) {
	rows, err := s.db.Query(`
		SELECT a.id,a.kind,a.record_id,a.file_name,a.mime,COALESCE(m.nickname,''),a.created_at
		FROM attachment a LEFT JOIN member m ON m.id=a.created_by
		WHERE a.kind=? AND a.record_id=? ORDER BY a.id DESC`, kind, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Attachment
	for rows.Next() {
		var v Attachment
		if err := rows.Scan(&v.ID, &v.Kind, &v.RecordID, &v.FileName, &v.Mime, &v.CreatedBy, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) AttachmentData(id int64) (Attachment, []byte, error) {
	var v Attachment
	var data []byte
	err := s.db.QueryRow(`
		SELECT a.id,a.kind,a.record_id,a.file_name,a.mime,a.data,a.created_at
		FROM attachment a WHERE a.id=?`, id).
		Scan(&v.ID, &v.Kind, &v.RecordID, &v.FileName, &v.Mime, &data, &v.CreatedAt)
	return v, data, err
}

func (s *Store) DeleteAttachment(id int64) error {
	_, err := s.db.Exec(`DELETE FROM attachment WHERE id=?`, id)
	return err
}

type BabyProfile struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Birthday string `json:"birthday,omitempty"`
	Note     string `json:"note,omitempty"`
	Active   bool   `json:"active"`
}

type BackupTask struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Note       string `json:"note"`
	Kind       string `json:"kind"`
	DueOn      string `json:"due_on"`
	AssigneeID int64  `json:"assignee_id"`
	Status     string `json:"status"`
	CreatedBy  int64  `json:"created_by"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

func (s *Store) AddBabyProfile(name, birthday, note string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("请填写宝宝昵称")
	}
	res, err := s.db.Exec(`INSERT INTO baby_profile(name,birthday,note) VALUES(?,?,?)`, name, strings.TrimSpace(birthday), strings.TrimSpace(note))
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	// 刚添加的宝宝直接设为当前宝宝，和前端「添加后切过去」的行为保持一致。
	_ = s.ActivateBaby(id)
	return id, nil
}

// ActivateBaby 把指定宝宝设为当前宝宝，其余一律置为 inactive。
// 原先 is_active 列每行 DEFAULT 1，所有宝宝都是 active，
// 「当前宝宝」这个概念实际是失效的，切换只存在前端内存里、刷新就丢。
func (s *Store) ActivateBaby(id int64) error {
	if id <= 0 {
		return errors.New("宝宝编号不合法")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM baby_profile WHERE id=?`, id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return errors.New("找不到这个宝宝档案")
	}
	if _, err := tx.Exec(`UPDATE baby_profile SET is_active=0 WHERE id<>?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE baby_profile SET is_active=1 WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// SyncBabyProfileFromProfile 把「设置」里改的宝宝昵称/生日同步到 1 号宝宝档案。
// 前端设置页写的是 profile.baby_name，而多宝宝页读的是 baby_profile.name，
// 两边不联动的话，用户在设置里改了名字，宝宝档案页显示的还是旧名字。
func (s *Store) SyncBabyProfileFromProfile(name, birthday string) error {
	name = strings.TrimSpace(name)
	birthday = strings.TrimSpace(birthday)
	if name == "" && birthday == "" {
		return nil
	}
	if name != "" {
		if _, err := s.db.Exec(`UPDATE baby_profile SET name=? WHERE id=1`, name); err != nil {
			return err
		}
	}
	if birthday != "" {
		if _, err := s.db.Exec(`UPDATE baby_profile SET birthday=? WHERE id=1`, birthday); err != nil {
			return err
		}
	}
	return nil
}

// ensureSingleActiveBaby 把历史上「全部 active」的数据收敛成只有一个当前宝宝
// （保留 id 最小的），否则老库升级后切换逻辑依然是乱的。
func (s *Store) ensureSingleActiveBaby() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM baby_profile WHERE is_active=1`).Scan(&n); err != nil {
		return err
	}
	if n <= 1 {
		return nil
	}
	var keep int64
	if err := s.db.QueryRow(`SELECT id FROM baby_profile WHERE is_active=1 ORDER BY id LIMIT 1`).Scan(&keep); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE baby_profile SET is_active=0 WHERE id<>?`, keep)
	return err
}

type BackupShopping struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	Category  string  `json:"category"`
	Qty       float64 `json:"qty"`
	Unit      string  `json:"unit"`
	Checked   int     `json:"checked"`
	BoughtBy  int64   `json:"bought_by"`
	Note      string  `json:"note"`
	CreatedBy int64   `json:"created_by"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}
type BackupHealth struct {
	ID        int64  `json:"id"`
	BabyID    int64  `json:"baby_id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	On        string `json:"occurred_on"`
	Value     string `json:"value"`
	Note      string `json:"note"`
	CreatedBy int64  `json:"created_by"`
	CreatedAt string `json:"created_at"`
}
type BackupReminder struct {
	ID         int64  `json:"id"`
	BabyID     int64  `json:"baby_id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	DueOn      string `json:"due_on"`
	RepeatDays int    `json:"repeat_days"`
	Status     string `json:"status"`
	Note       string `json:"note"`
	CreatedBy  int64  `json:"created_by"`
	CreatedAt  string `json:"created_at"`
}
type BackupAttachment struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	RecordID  int64  `json:"record_id"`
	FileName  string `json:"file_name"`
	Mime      string `json:"mime"`
	Data      []byte `json:"data"`
	CreatedBy int64  `json:"created_by"`
	CreatedAt string `json:"created_at"`
}
type BackupAudit struct {
	ID        int64  `json:"id"`
	Action    string `json:"action"`
	Entity    string `json:"entity"`
	EntityID  int64  `json:"entity_id"`
	Detail    string `json:"detail"`
	MemberID  int64  `json:"member_id"`
	CreatedAt string `json:"created_at"`
}

func (s *Store) ListBabies() ([]BabyProfile, error) {
	rows, err := s.db.Query(`SELECT id,name,COALESCE(birthday,''),COALESCE(note,''),is_active FROM baby_profile ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BabyProfile
	for rows.Next() {
		var v BabyProfile
		var active int
		if err := rows.Scan(&v.ID, &v.Name, &v.Birthday, &v.Note, &active); err != nil {
			return nil, err
		}
		v.Active = active != 0
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) UpdateBabyProfile(id int64, name, birthday, note string) error {
	if id <= 0 || strings.TrimSpace(name) == "" {
		return errors.New("宝宝档案不合法")
	}
	_, err := s.db.Exec(`UPDATE baby_profile SET name=?,birthday=?,note=? WHERE id=?`, strings.TrimSpace(name), strings.TrimSpace(birthday), strings.TrimSpace(note), id)
	return err
}

type AnnualReport struct {
	Year       string      `json:"year"`
	GiftRecv   int64       `json:"gift_recv"`
	GiftSent   int64       `json:"gift_sent"`
	BabyTotal  int64       `json:"baby_total"`
	GiftCount  int         `json:"gift_count"`
	BabyCount  int         `json:"baby_count"`
	People     int         `json:"people"`
	Categories []CatSum    `json:"categories"`
	Months     []MonthSum  `json:"months"`
	TopPersons []CatSum    `json:"top_persons"`
	Milestones []Milestone `json:"milestones"`
}

func (s *Store) ListBabyProfilesForBackup() ([]BabyProfile, error) { return s.ListBabies() }

func (s *Store) AnnualReport(year string) (AnnualReport, error) {
	return s.AnnualReportFor(year, 0)
}

func (s *Store) AnnualReportFor(year string, babyID int64) (AnnualReport, error) {
	if len(year) != 4 {
		year = time.Now().Format("2006")
	}
	from, to := year+"-01-01", year+"-12-31"
	v := AnnualReport{Year: year}
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(CASE WHEN direction='in' THEN amount_cents END),0),COALESCE(SUM(CASE WHEN direction='out' THEN amount_cents END),0),COUNT(*),COUNT(DISTINCT person_id) FROM gift_record WHERE occurred_on BETWEEN ? AND ?`, from, to).Scan(&v.GiftRecv, &v.GiftSent, &v.GiftCount, &v.People); err != nil {
		return v, err
	}
	babyClause, babyArgs := "", []any{}
	if babyID > 0 { babyClause = ` AND baby_id=?`; babyArgs = []any{babyID} }
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(amount_cents),0),COUNT(*) FROM baby_expense WHERE occurred_on BETWEEN ? AND ?`+babyClause, append([]any{from, to}, babyArgs...)...).Scan(&v.BabyTotal, &v.BabyCount); err != nil {
		return v, err
	}
	if rows, err := s.db.Query(`SELECT category,SUM(amount_cents),COUNT(*) FROM baby_expense WHERE occurred_on BETWEEN ? AND ?`+babyClause+` GROUP BY category ORDER BY 2 DESC`, append([]any{from, to}, babyArgs...)...); err == nil {
		defer rows.Close()
		for rows.Next() {
			var c CatSum
			if err := rows.Scan(&c.Category, &c.Cents, &c.Count); err != nil {
				return v, err
			}
			v.Categories = append(v.Categories, c)
		}
	}
	if rows, err := s.db.Query(`SELECT strftime('%Y-%m',occurred_on),SUM(amount_cents),COUNT(*) FROM baby_expense WHERE occurred_on BETWEEN ? AND ?`+babyClause+` GROUP BY 1 ORDER BY 1`, append([]any{from, to}, babyArgs...)...); err == nil {
		defer rows.Close()
		for rows.Next() {
			var m MonthSum
			if err := rows.Scan(&m.Month, &m.Cents, &m.Count); err != nil {
				return v, err
			}
			v.Months = append(v.Months, m)
		}
	}
	if rows, err := s.db.Query(`SELECT p.name,SUM(r.amount_cents),COUNT(*) FROM gift_record r JOIN person p ON p.id=r.person_id WHERE r.occurred_on BETWEEN ? AND ? GROUP BY p.id ORDER BY 2 DESC LIMIT 10`, from, to); err == nil {
		defer rows.Close()
		for rows.Next() {
			var c CatSum
			if err := rows.Scan(&c.Category, &c.Cents, &c.Count); err != nil {
				return v, err
			}
			v.TopPersons = append(v.TopPersons, c)
		}
	}
	all, err := s.ListMilestonesFor(babyID, 500)
	if err != nil {
		return v, err
	}
	for _, m := range all {
		if strings.HasPrefix(m.On, year+"-") {
			v.Milestones = append(v.Milestones, m)
		}
	}
	return v, nil
}

// csvHeaderCells 是导出时用的表头文案，用来判断首行到底是不是表头。
var csvHeaderCells = map[string]bool{
	"日期": true, "姓名": true, "关系": true, "方向": true, "金额": true, "金额(元)": true,
	"事由": true, "方式": true, "备注": true, "记录人": true, "分类": true,
	"数量": true, "单位": true, "商家": true,
}

// looksLikeHeader 判断首行是否为表头。以前无脑跳过第 0 行，
// 导致从别处导出的、没有表头的 CSV 会静默丢掉第一条记录。
func looksLikeHeader(row []string) bool {
	for _, c := range row {
		if csvHeaderCells[strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(c), "\uFEFF"))] {
			return true
		}
	}
	return false
}

func (s *Store) ImportCSVRows(book string, rows [][]string, by int64) (int, error) {
	count := 0
	for i, row := range rows {
		if len(row) < 3 {
			continue
		}
		if i == 0 {
			row[0] = strings.TrimPrefix(row[0], "\uFEFF") // 去掉 Excel 导出的 BOM
			if looksLikeHeader(row) {
				continue
			}
		}
		if book == "baby" {
			if _, err := s.AddBaby(field(row, 1), field(row, 2), field(row, 0), field(row, 5), field(row, 6), parseQty(field(row, 3)), field(row, 4), by); err == nil {
				count++
			}
			continue
		}
		pid, err := s.EnsurePerson(field(row, 1), field(row, 2), "")
		if err != nil {
			continue
		}
		eid := int64(0)
		if field(row, 5) != "" {
			eid, _ = s.EnsureEvent(field(row, 5))
		}
		if _, err := s.AddGift(pid, eid, importDirection(field(row, 3)), field(row, 4), field(row, 0), field(row, 6), field(row, 7), by); err == nil {
			count++
		}
	}
	return count, nil
}

func field(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func parseQty(v string) float64 {
	var out float64
	_, _ = fmt.Sscanf(strings.ReplaceAll(v, ",", ""), "%f", &out)
	return out
}

func importDirection(v string) string {
	if strings.Contains(v, "收") || strings.EqualFold(v, "in") {
		return "in"
	}
	return "out"
}
