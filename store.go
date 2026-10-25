package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// 所有金额均以「分」为单位的整数存储，绝不使用浮点数。

const timeLayout = "2006-01-02 15:04:05"

const schema = `
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS member (
  id         INTEGER PRIMARY KEY,
  nickname   TEXT NOT NULL UNIQUE,
  is_active  INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
);

CREATE TABLE IF NOT EXISTS app_token (
  token      TEXT PRIMARY KEY,
  member_id  INTEGER NOT NULL REFERENCES member(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  expires_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_token_member ON app_token(member_id);

CREATE TABLE IF NOT EXISTS person (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE COLLATE NOCASE,
  py         TEXT,
  relation   TEXT,
  note       TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
);

CREATE TABLE IF NOT EXISTS gift_event (
  id          INTEGER PRIMARY KEY,
  name        TEXT NOT NULL UNIQUE COLLATE NOCASE,
  event_date  TEXT,
  note        TEXT,
  created_at  TEXT NOT NULL DEFAULT (datetime('now','localtime'))
);

CREATE TABLE IF NOT EXISTS gift_record (
  id           INTEGER PRIMARY KEY,
  person_id    INTEGER NOT NULL REFERENCES person(id) ON DELETE RESTRICT,
  event_id     INTEGER REFERENCES gift_event(id) ON DELETE SET NULL,
  direction    TEXT NOT NULL CHECK (direction IN ('in','out')),
  amount_cents INTEGER NOT NULL CHECK (amount_cents > 0),
  occurred_on  TEXT NOT NULL DEFAULT (date('now','localtime')),
  method       TEXT,
  note         TEXT,
  created_by   INTEGER REFERENCES member(id) ON DELETE SET NULL,
  created_at   TEXT NOT NULL DEFAULT (datetime('now','localtime'))
);
CREATE INDEX IF NOT EXISTS idx_gr_person ON gift_record(person_id, occurred_on DESC);
CREATE INDEX IF NOT EXISTS idx_gr_date   ON gift_record(occurred_on DESC);
CREATE INDEX IF NOT EXISTS idx_gr_event  ON gift_record(event_id);

CREATE TABLE IF NOT EXISTS baby_expense (
  id           INTEGER PRIMARY KEY,
  category     TEXT NOT NULL,
  amount_cents INTEGER NOT NULL CHECK (amount_cents > 0),
  occurred_on  TEXT NOT NULL DEFAULT (date('now','localtime')),
  merchant     TEXT,
  note         TEXT,
  created_by   INTEGER REFERENCES member(id) ON DELETE SET NULL,
  created_at   TEXT NOT NULL DEFAULT (datetime('now','localtime'))
);
CREATE INDEX IF NOT EXISTS idx_be_date ON baby_expense(occurred_on DESC);
CREATE INDEX IF NOT EXISTS idx_be_cat  ON baby_expense(category, occurred_on DESC);

CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT
);
`

// Store 持有数据库连接。
type Store struct {
	db *sql.DB
}

// Open 打开（必要时创建）数据库并执行迁移。
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	s := &Store{db: db}
	if err := s.upgrade(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.seed(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// upgrade 增量迁移：老库平滑升级，不会丢数据。
func (s *Store) upgrade() error {
	if !s.hasColumn("baby_expense", "qty") {
		if _, err := s.db.Exec(`ALTER TABLE baby_expense ADD COLUMN qty REAL`); err != nil {
			return err
		}
	}
	if !s.hasColumn("baby_expense", "unit") {
		if _, err := s.db.Exec(`ALTER TABLE baby_expense ADD COLUMN unit TEXT`); err != nil {
			return err
		}
	}
	// settle: null/'' 未处理 · done 已回礼 · skip 无需回礼（如长辈压岁钱）
	if !s.hasColumn("gift_record", "settle") {
		if _, err := s.db.Exec(`ALTER TABLE gift_record ADD COLUMN settle TEXT`); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS milestone (
		  id          INTEGER PRIMARY KEY,
		  kind        TEXT NOT NULL,
		  title       TEXT NOT NULL,
		  occurred_on TEXT NOT NULL DEFAULT (date('now','localtime')),
		  note        TEXT,
		  created_by  INTEGER REFERENCES member(id) ON DELETE SET NULL,
		  created_at  TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		);
		CREATE INDEX IF NOT EXISTS idx_ms_date ON milestone(occurred_on DESC);
	`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS profile (
		  key   TEXT PRIMARY KEY,
		  value TEXT
		);`); err != nil {
		return err
	}
	// 宝宝生日，用于按月龄统计与换算
	for _, kv := range [][2]string{
		{"baby_name", "宝宝"},
		{"baby_birthday", ""},
	} {
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO profile(key,value) VALUES (?,?)`, kv[0], kv[1]); err != nil {
			return err
		}
	}
	// 第二阶段：事件工作台、家庭协作、附件、操作记录和多宝宝基础维度。
	for _, migration := range []struct{ table, column, ddl string }{
		{"gift_event", "status", `ALTER TABLE gift_event ADD COLUMN status TEXT NOT NULL DEFAULT 'planned'`},
		{"gift_event", "venue", `ALTER TABLE gift_event ADD COLUMN venue TEXT`},
		{"gift_event", "budget_cents", `ALTER TABLE gift_event ADD COLUMN budget_cents INTEGER NOT NULL DEFAULT 0`},
		{"gift_event", "guest_count", `ALTER TABLE gift_event ADD COLUMN guest_count INTEGER NOT NULL DEFAULT 0`},
		{"gift_record", "thanks_status", `ALTER TABLE gift_record ADD COLUMN thanks_status TEXT NOT NULL DEFAULT 'pending'`},
		{"baby_expense", "baby_id", `ALTER TABLE baby_expense ADD COLUMN baby_id INTEGER NOT NULL DEFAULT 1`},
		{"milestone", "baby_id", `ALTER TABLE milestone ADD COLUMN baby_id INTEGER NOT NULL DEFAULT 1`},
	} {
		if !s.hasColumn(migration.table, migration.column) {
			if _, err := s.db.Exec(migration.ddl); err != nil {
				return err
			}
		}
	}
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS baby_profile (
		  id         INTEGER PRIMARY KEY,
		  name       TEXT NOT NULL,
		  birthday   TEXT,
		  note       TEXT,
		  is_active  INTEGER NOT NULL DEFAULT 1,
		  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		);
		CREATE TABLE IF NOT EXISTS family_task (
		  id          INTEGER PRIMARY KEY,
		  title       TEXT NOT NULL,
		  note        TEXT,
		  kind        TEXT NOT NULL DEFAULT '家庭事项',
		  due_on      TEXT,
		  assignee_id INTEGER REFERENCES member(id) ON DELETE SET NULL,
		  status      TEXT NOT NULL DEFAULT 'open',
		  created_by  INTEGER REFERENCES member(id) ON DELETE SET NULL,
		  created_at  TEXT NOT NULL DEFAULT (datetime('now','localtime')),
		  updated_at  TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		);
		CREATE INDEX IF NOT EXISTS idx_task_status_due ON family_task(status, due_on);
		CREATE TABLE IF NOT EXISTS shopping_item (
		  id          INTEGER PRIMARY KEY,
		  name        TEXT NOT NULL,
		  category    TEXT,
		  qty         REAL,
		  unit        TEXT,
		  checked     INTEGER NOT NULL DEFAULT 0,
		  bought_by   INTEGER REFERENCES member(id) ON DELETE SET NULL,
		  note        TEXT,
		  created_by  INTEGER REFERENCES member(id) ON DELETE SET NULL,
		  created_at  TEXT NOT NULL DEFAULT (datetime('now','localtime')),
		  updated_at  TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		);
		CREATE INDEX IF NOT EXISTS idx_shop_checked ON shopping_item(checked, created_at DESC);
		CREATE TABLE IF NOT EXISTS attachment (
		  id          INTEGER PRIMARY KEY,
		  kind        TEXT NOT NULL,
		  record_id   INTEGER NOT NULL,
		  file_name   TEXT NOT NULL,
		  mime        TEXT NOT NULL,
		  data        BLOB NOT NULL,
		  created_by  INTEGER REFERENCES member(id) ON DELETE SET NULL,
		  created_at  TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		);
		CREATE INDEX IF NOT EXISTS idx_attachment_record ON attachment(kind, record_id);
		CREATE TABLE IF NOT EXISTS audit_log (
		  id          INTEGER PRIMARY KEY,
		  action      TEXT NOT NULL,
		  entity      TEXT NOT NULL,
		  entity_id   INTEGER,
		  detail      TEXT,
		  member_id   INTEGER REFERENCES member(id) ON DELETE SET NULL,
		  created_at  TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		);
		CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_log(created_at DESC);
		CREATE TABLE IF NOT EXISTS health_record (
		  id          INTEGER PRIMARY KEY,
		  baby_id     INTEGER NOT NULL DEFAULT 1,
		  kind        TEXT NOT NULL,
		  title       TEXT NOT NULL,
		  occurred_on TEXT NOT NULL DEFAULT (date('now','localtime')),
		  value       TEXT,
		  note        TEXT,
		  created_by  INTEGER REFERENCES member(id) ON DELETE SET NULL,
		  created_at  TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		);
		CREATE INDEX IF NOT EXISTS idx_health_baby_date ON health_record(baby_id, occurred_on DESC);
		CREATE TABLE IF NOT EXISTS reminder (
		  id          INTEGER PRIMARY KEY,
		  baby_id     INTEGER NOT NULL DEFAULT 1,
		  kind        TEXT NOT NULL,
		  title       TEXT NOT NULL,
		  due_on      TEXT NOT NULL,
		  repeat_days INTEGER NOT NULL DEFAULT 0,
		  status      TEXT NOT NULL DEFAULT 'open',
		  note        TEXT,
		  created_by  INTEGER REFERENCES member(id) ON DELETE SET NULL,
		  created_at  TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		);
		CREATE INDEX IF NOT EXISTS idx_reminder_due ON reminder(status, due_on);
	`); err != nil {
		return err
	}
	// 老库首次升级时 baby_name 往往还没设置，直接播种会得到一个没有名字的宝宝档案。
	babyName := strings.TrimSpace(s.GetProfile("baby_name"))
	if babyName == "" {
		babyName = "宝宝"
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO baby_profile(id,name,birthday) VALUES (1,?,?)`, babyName, s.GetProfile("baby_birthday")); err != nil {
		return err
	}
	// 历史上 is_active 每行都是 1（全部宝宝都 active），这里收敛成只有一个当前宝宝。
	if err := s.ensureSingleActiveBaby(); err != nil {
		return err
	}
	return nil
}

func (s *Store) hasColumn(table, col string) bool {
	rows, err := s.db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			continue
		}
		if strings.EqualFold(name, col) {
			return true
		}
	}
	return false
}

func (s *Store) GetProfile(k string) string {
	var v string
	_ = s.db.QueryRow(`SELECT value FROM profile WHERE key=?`, k).Scan(&v)
	return v
}

func (s *Store) SetProfile(k, v string) error {
	_, err := s.db.Exec(`INSERT INTO profile(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, k, v)
	return err
}

func (s *Store) Close() error { return s.db.Close() }

// seed 写入默认成员与常用事由，避免首次使用时一片空白。
func (s *Store) seed() error {
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO member(id, nickname) VALUES (1,'爸爸'),(2,'妈妈')`); err != nil {
		return err
	}
	for _, e := range []string{"满月酒", "百日宴", "周岁宴", "春节红包", "生日礼", "婚礼", "乔迁", "日常往来"} {
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO gift_event(name) VALUES (?)`, e); err != nil {
			return err
		}
	}
	return nil
}

// ---- 通用工具 ----

// ParseMoney 把用户输入的金额字符串转成分。支持 "600"、"600.5"、"1,200"、"¥600"。
func ParseMoney(in string) (int64, error) {
	s := strings.TrimSpace(in)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "¥", "")
	s = strings.ReplaceAll(s, "￥", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("金额为空")
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	var yuan, cent string
	if i := strings.IndexByte(s, '.'); i >= 0 {
		yuan, cent = s[:i], s[i+1:]
		if len(cent) > 2 {
			cent = cent[:2]
		}
		for len(cent) < 2 {
			cent += "0"
		}
	} else {
		yuan = s
		cent = "00"
	}
	if yuan == "" {
		yuan = "0"
	}
	var y, c int64
	for _, r := range yuan {
		if r < '0' || r > '9' {
			return 0, errors.New("金额格式不对")
		}
		y = y*10 + int64(r-'0')
	}
	for _, r := range cent {
		if r < '0' || r > '9' {
			return 0, errors.New("金额格式不对")
		}
		c = c*10 + int64(r-'0')
	}
	v := y*100 + c
	if v <= 0 {
		return 0, errors.New("金额必须大于 0")
	}
	if neg {
		v = -v
	}
	return v, nil
}

func today() string { return time.Now().Format("2006-01-02") }

func validDate(d string) string {
	if d == "" {
		return today()
	}
	if _, err := time.Parse("2006-01-02", d); err != nil {
		return today()
	}
	return d
}

// ---- 成员 ----

type Member struct {
	ID       int64  `json:"id"`
	Nickname string `json:"nickname"`
}

func (s *Store) ListMembers() ([]Member, error) {
	rows, err := s.db.Query(`SELECT id, nickname FROM member WHERE is_active=1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.Nickname); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) AddMember(nickname string) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO member(nickname) VALUES (?)`, strings.TrimSpace(nickname))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ---- Token ----

func (s *Store) CreateToken(memberID int64, days int) (string, error) {
	tk, err := randomToken()
	if err != nil {
		return "", err
	}
	exp := time.Now().AddDate(0, 0, days).Format("2006-01-02 15:04:05")
	if _, err := s.db.Exec(`INSERT INTO app_token(token, member_id, expires_at) VALUES (?,?,?)`, tk, memberID, exp); err != nil {
		return "", err
	}
	// 顺手清理过期
	s.db.Exec(`DELETE FROM app_token WHERE expires_at < datetime('now','localtime')`)
	return tk, nil
}

func (s *Store) MemberByToken(tk string) (*Member, error) {
	var m Member
	err := s.db.QueryRow(`
		SELECT m.id, m.nickname FROM app_token t
		JOIN member m ON m.id=t.member_id
		WHERE t.token=? AND t.expires_at >= datetime('now','localtime') AND m.is_active=1`, tk).Scan(&m.ID, &m.Nickname)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) RotateTokens() error {
	_, err := s.db.Exec(`DELETE FROM app_token`)
	return err
}

// ---- 往来对象 ----

type Person struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Py        string `json:"py,omitempty"`
	Relation  string `json:"relation,omitempty"`
	Note      string `json:"note,omitempty"`
	RecvCents int64  `json:"recv_cents"`
	SentCents int64  `json:"sent_cents"`
	NetCents  int64  `json:"net_cents"` // >0 表示我方净收入，即「还欠对方人情」
	Count     int    `json:"count"`
	LastOn    string `json:"last_on,omitempty"`
	LastInOn  string `json:"last_in_on,omitempty"` // 最后一次收到礼金的日期，用于算"欠了多久"
	LastDir   string `json:"last_dir,omitempty"`
	LastAmt   int64  `json:"last_cents,omitempty"`
	LastEvent string `json:"last_event,omitempty"`
}

const personSelect = `
SELECT p.id, p.name, COALESCE(p.py,''), COALESCE(p.relation,''), COALESCE(p.note,''),
  COALESCE((SELECT SUM(amount_cents) FROM gift_record r WHERE r.person_id=p.id AND r.direction='in'),0)  AS recv_cents,
  COALESCE((SELECT SUM(amount_cents) FROM gift_record r WHERE r.person_id=p.id AND r.direction='out'),0) AS sent_cents,
  COALESCE((SELECT COUNT(*) FROM gift_record r WHERE r.person_id=p.id),0)                                AS cnt,
  COALESCE((SELECT MAX(occurred_on) FROM gift_record r WHERE r.person_id=p.id),'')                       AS last_on,
  COALESCE((SELECT MAX(occurred_on) FROM gift_record r WHERE r.person_id=p.id AND r.direction='in'),'')  AS last_in_on,
  COALESCE(lr.direction,''), COALESCE(lr.amount_cents,0), COALESCE(e.name,'')
FROM person p
LEFT JOIN gift_record lr ON lr.id = (
    SELECT id FROM gift_record WHERE person_id=p.id ORDER BY occurred_on DESC, id DESC LIMIT 1
)
LEFT JOIN gift_event e ON e.id = lr.event_id
`

func scanPerson(rows *sql.Rows) (Person, error) {
	var p Person
	var note string
	err := rows.Scan(&p.ID, &p.Name, &p.Py, &p.Relation, &note,
		&p.RecvCents, &p.SentCents, &p.Count, &p.LastOn, &p.LastInOn, &p.LastDir, &p.LastAmt, &p.LastEvent)
	p.Note = note
	p.NetCents = p.RecvCents - p.SentCents
	return p, err
}

// SearchPersons 按关键字搜索（命中姓名、拼音、关系），按最近往来时间排序。
func (s *Store) SearchPersons(q string, limit int) ([]Person, error) {
	if limit <= 0 {
		limit = 50
	}
	kw := strings.TrimSpace(q)
	var rows *sql.Rows
	var err error
	if kw == "" {
		rows, err = s.db.Query(personSelect+` ORDER BY last_on DESC, p.id DESC LIMIT ?`, limit)
	} else {
		like := "%" + kw + "%"
		rows, err = s.db.Query(personSelect+`
			WHERE p.name LIKE ? OR p.py LIKE ? OR p.relation LIKE ? OR p.note LIKE ?
			ORDER BY (p.name = ?) DESC, last_on DESC, p.id DESC LIMIT ?`,
			like, like, like, like, kw, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Person
	for rows.Next() {
		p, err := scanPerson(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetPerson(id int64) (*Person, error) {
	rows, err := s.db.Query(personSelect+` WHERE p.id=?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		p, err := scanPerson(rows)
		if err != nil {
			return nil, err
		}
		return &p, nil
	}
	return nil, nil
}

func (s *Store) EnsurePerson(name, relation, py string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("请填写姓名")
	}
	res, err := s.db.Exec(`INSERT INTO person(name, relation, py) VALUES (?,?,?)`, name, relation, py)
	if err == nil {
		id, _ := res.LastInsertId()
		return id, nil
	}
	// 已存在则返回既有 ID
	var id int64
	err2 := s.db.QueryRow(`SELECT id FROM person WHERE name=?`, name).Scan(&id)
	if err2 != nil {
		return 0, err
	}
	if relation != "" {
		s.db.Exec(`UPDATE person SET relation=? WHERE id=? AND (relation IS NULL OR relation='')`, relation, id)
	}
	return id, nil
}

func (s *Store) UpdatePerson(id int64, name, relation, note string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("姓名不能为空")
	}
	_, err := s.db.Exec(`UPDATE person SET name=?, relation=?, note=? WHERE id=?`, name, relation, note, id)
	return err
}

func (s *Store) DeletePerson(id int64) error {
	n, err := s.count(`SELECT COUNT(*) FROM gift_record WHERE person_id=?`, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("该联系人已有 %d 笔往来记录，不能删除", n)
	}
	_, err = s.db.Exec(`DELETE FROM person WHERE id=?`, id)
	return err
}

func (s *Store) count(q string, args ...any) (int, error) {
	var n int
	err := s.db.QueryRow(q, args...).Scan(&n)
	return n, err
}

// ---- 人情记录 ----

type Gift struct {
	ID        int64  `json:"id"`
	PersonID  int64  `json:"person_id"`
	Person    string `json:"person_name"`
	EventID   int64  `json:"event_id,omitempty"`
	Event     string `json:"event_name,omitempty"`
	Direction string `json:"direction"`
	Cents     int64  `json:"amount_cents"`
	On        string `json:"occurred_on"`
	Method    string `json:"method,omitempty"`
	Note      string `json:"note,omitempty"`
	Settle    string `json:"settle,omitempty"`
	Thanks    string `json:"thanks_status,omitempty"`
	CreatedBy string `json:"created_by,omitempty"`
}

func (s *Store) AddGift(personID, eventID int64, dir, amountStr, date, method, note string, byID int64) (int64, error) {
	cents, err := ParseMoney(amountStr)
	if err != nil {
		return 0, err
	}
	if cents <= 0 {
		return 0, errors.New("金额必须大于 0")
	}
	if dir != "in" && dir != "out" {
		return 0, errors.New("方向必须是 in 或 out")
	}
	var ev any
	if eventID > 0 {
		ev = eventID
	}
	res, err := s.db.Exec(`
		INSERT INTO gift_record(person_id, event_id, direction, amount_cents, occurred_on, method, note, created_by)
		VALUES (?,?,?,?,?,?,?,?)`, personID, ev, dir, cents, validDate(date), method, note, nullID(byID))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) DeleteGift(id int64) error {
	_, err := s.db.Exec(`DELETE FROM gift_record WHERE id=?`, id)
	return err
}

func (s *Store) ListGifts(personID int64, limit int) ([]Gift, error) {
	if limit <= 0 {
		limit = 200
	}
	base := `
	SELECT r.id, r.person_id, p.name, COALESCE(r.event_id,0), COALESCE(e.name,''),
	       r.direction, r.amount_cents, r.occurred_on, COALESCE(r.method,''), COALESCE(r.note,''),
	       COALESCE(r.settle,''), COALESCE(r.thanks_status,'pending'), COALESCE(m.nickname,'')
	FROM gift_record r
	JOIN person p ON p.id = r.person_id
	LEFT JOIN gift_event e ON e.id = r.event_id
	LEFT JOIN member m ON m.id = r.created_by
	`
	var rows *sql.Rows
	var err error
	if personID > 0 {
		rows, err = s.db.Query(base+` WHERE r.person_id=? ORDER BY r.occurred_on DESC, r.id DESC LIMIT ?`, personID, limit)
	} else {
		rows, err = s.db.Query(base+` ORDER BY r.occurred_on DESC, r.id DESC LIMIT ?`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Gift
	for rows.Next() {
		var g Gift
		if err := rows.Scan(&g.ID, &g.PersonID, &g.Person, &g.EventID, &g.Event, &g.Direction, &g.Cents, &g.On, &g.Method, &g.Note, &g.Settle, &g.Thanks, &g.CreatedBy); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

type GiftSummary struct {
	RecvCents int64 `json:"recv_cents"`
	SentCents int64 `json:"sent_cents"`
	NetCents  int64 `json:"net_cents"`
	Count     int   `json:"count"`
	Persons   int   `json:"persons"`
}

func (s *Store) GiftSummary() (GiftSummary, error) {
	var v GiftSummary
	err := s.db.QueryRow(`
		SELECT COALESCE(SUM(CASE WHEN direction='in'  THEN amount_cents END),0),
		       COALESCE(SUM(CASE WHEN direction='out' THEN amount_cents END),0),
		       COUNT(*),
		       (SELECT COUNT(DISTINCT person_id) FROM gift_record)
		FROM gift_record`).Scan(&v.RecvCents, &v.SentCents, &v.Count, &v.Persons)
	v.NetCents = v.RecvCents - v.SentCents
	return v, err
}

// OweList 返回「我还没还完」的清单：收大于送的人。
func (s *Store) OweList(limit int) ([]Person, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(personSelect+`
		WHERE COALESCE((SELECT SUM(amount_cents) FROM gift_record r WHERE r.person_id=p.id AND r.direction='in' AND COALESCE(r.settle,'')=''),0)
		    > COALESCE((SELECT SUM(amount_cents) FROM gift_record r WHERE r.person_id=p.id AND r.direction='out'),0)
		ORDER BY (COALESCE((SELECT SUM(amount_cents) FROM gift_record r WHERE r.person_id=p.id AND r.direction='in' AND COALESCE(r.settle,'')=''),0)
		        - COALESCE((SELECT SUM(amount_cents) FROM gift_record r WHERE r.person_id=p.id AND r.direction='out'),0)) DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Person
	for rows.Next() {
		p, err := scanPerson(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---- 事由 ----

type Event struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Date        string `json:"event_date,omitempty"`
	Note        string `json:"note,omitempty"`
	Status      string `json:"status"`
	Venue       string `json:"venue,omitempty"`
	BudgetCents int64  `json:"budget_cents,omitempty"`
	GuestCount  int    `json:"guest_count,omitempty"`
	Cnt         int    `json:"count"`
}

func (s *Store) ListEvents() ([]Event, error) {
	rows, err := s.db.Query(`
		SELECT e.id, e.name, COALESCE(e.event_date,''), COALESCE(e.note,''), COALESCE(e.status,'planned'),
		       COALESCE(e.venue,''), COALESCE(e.budget_cents,0), COALESCE(e.guest_count,0),
		       COALESCE((SELECT COUNT(*) FROM gift_record r WHERE r.event_id=e.id),0) AS cnt
		FROM gift_event e ORDER BY cnt DESC, e.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Name, &e.Date, &e.Note, &e.Status, &e.Venue, &e.BudgetCents, &e.GuestCount, &e.Cnt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) EnsureEvent(name string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("事由为空")
	}
	res, err := s.db.Exec(`INSERT INTO gift_event(name) VALUES (?)`, name)
	if err == nil {
		id, _ := res.LastInsertId()
		return id, nil
	}
	var id int64
	if err := s.db.QueryRow(`SELECT id FROM gift_event WHERE name=?`, name).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// ---- 添丁记 ----

type BabyExp struct {
	ID        int64   `json:"id"`
	BabyID    int64   `json:"baby_id"`
	Category  string  `json:"category"`
	Cents     int64   `json:"amount_cents"`
	On        string  `json:"occurred_on"`
	Merchant  string  `json:"merchant,omitempty"`
	Qty       float64 `json:"qty,omitempty"`
	Unit      string  `json:"unit,omitempty"`
	Note      string  `json:"note,omitempty"`
	CreatedBy string  `json:"created_by,omitempty"`
}

func (s *Store) AddBaby(cat, amountStr, date, merchant, note string, qty float64, unit string, byID int64) (int64, error) {
	return s.AddBabyFor(1, cat, amountStr, date, merchant, note, qty, unit, byID)
}

func (s *Store) AddBabyFor(babyID int64, cat, amountStr, date, merchant, note string, qty float64, unit string, byID int64) (int64, error) {
	cents, err := ParseMoney(amountStr)
	if err != nil {
		return 0, err
	}
	if cents <= 0 {
		return 0, errors.New("金额必须大于 0")
	}
	if strings.TrimSpace(cat) == "" {
		cat = "其他"
	}
	if babyID <= 0 {
		babyID = 1
	}
	var q any
	if qty > 0 {
		q = qty
	}
	res, err := s.db.Exec(`
		INSERT INTO baby_expense(baby_id, category, amount_cents, occurred_on, merchant, note, qty, unit, created_by)
		VALUES (?,?,?,?,?,?,?,?,?)`, babyID, cat, cents, validDate(date), merchant, note, q, strings.TrimSpace(unit), nullID(byID))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) DeleteBaby(id int64) error {
	_, err := s.db.Exec(`DELETE FROM baby_expense WHERE id=?`, id)
	return err
}

func (s *Store) ListBaby(limit int) ([]BabyExp, error) {
	return s.ListBabyFor(0, limit)
}

func (s *Store) ListBabyFor(babyID int64, limit int) ([]BabyExp, error) {
	if limit <= 0 {
		limit = 200
	}
	query := `
		SELECT b.id, COALESCE(b.baby_id,1), b.category, b.amount_cents, b.occurred_on, COALESCE(b.merchant,''), COALESCE(b.note,''),
		       COALESCE(b.qty,0), COALESCE(b.unit,''), COALESCE(m.nickname,'')
		FROM baby_expense b LEFT JOIN member m ON m.id=b.created_by
	`
	args := []any{}
	if babyID > 0 {
		query += ` WHERE b.baby_id=?`
		args = append(args, babyID)
	}
	query += ` ORDER BY b.occurred_on DESC, b.id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BabyExp
	for rows.Next() {
		var b BabyExp
		if err := rows.Scan(&b.ID, &b.BabyID, &b.Category, &b.Cents, &b.On, &b.Merchant, &b.Note, &b.Qty, &b.Unit, &b.CreatedBy); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

type CatSum struct {
	Category string `json:"category"`
	Cents    int64  `json:"amount_cents"`
	Count    int    `json:"count"`
}

type MonthSum struct {
	Month string `json:"month"`
	Cents int64  `json:"amount_cents"`
	Count int    `json:"count"`
}

type BabySummary struct {
	TotalCents    int64      `json:"total_cents"`
	Count         int        `json:"count"`
	Months        []MonthSum `json:"months"`
	Categories    []CatSum   `json:"categories"`
	CurMonthCents int64      `json:"cur_month_cents"`
}

func (s *Store) BabySummary(months int) (BabySummary, error) {
	return s.BabySummaryFor(0, months)
}

func (s *Store) BabySummaryFor(babyID int64, months int) (BabySummary, error) {
	if months <= 0 {
		months = 6
	}
	var v BabySummary
	where, args := "", []any{}
	if babyID > 0 {
		where = ` WHERE baby_id=?`
		args = append(args, babyID)
	}
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(amount_cents),0), COUNT(*) FROM baby_expense`+where, args...).Scan(&v.TotalCents, &v.Count); err != nil {
		return v, err
	}
	curWhere := ` WHERE strftime('%Y-%m',occurred_on)=strftime('%Y-%m','now','localtime')`
	curArgs := []any{}
	if babyID > 0 {
		curWhere += ` AND baby_id=?`
		curArgs = append(curArgs, babyID)
	}
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(amount_cents),0) FROM baby_expense`+curWhere, curArgs...).Scan(&v.CurMonthCents); err != nil {
		return v, err
	}
	monthQuery := `
		SELECT strftime('%Y-%m', occurred_on) ym, SUM(amount_cents), COUNT(*)
		FROM baby_expense` + where + ` GROUP BY ym ORDER BY ym DESC LIMIT ?`
	monthArgs := append(append([]any{}, args...), months)
	mrows, err := s.db.Query(monthQuery, monthArgs...)
	if err != nil {
		return v, err
	}
	defer mrows.Close()
	for mrows.Next() {
		var m MonthSum
		if err := mrows.Scan(&m.Month, &m.Cents, &m.Count); err != nil {
			return v, err
		}
		v.Months = append(v.Months, m)
	}
	catQuery := `
		SELECT category, SUM(amount_cents), COUNT(*) FROM baby_expense
		` + where + ` GROUP BY category ORDER BY 2 DESC`
	crows, err := s.db.Query(catQuery, args...)
	if err != nil {
		return v, err
	}
	defer crows.Close()
	for crows.Next() {
		var c CatSum
		if err := crows.Scan(&c.Category, &c.Cents, &c.Count); err != nil {
			return v, err
		}
		v.Categories = append(v.Categories, c)
	}
	return v, mrows.Err()
}

/* ==================== 编辑 / 批量录入 ==================== */

type GiftPatch struct {
	Direction string
	Amount    string
	PersonID  int64
	EventID   int64
	Date      string
	Method    string
	Note      string
	Settle    string
	Thanks    string
}

// UpdateGift 支持改金额、方向、日期、事由、备注、回礼状态；person_id 变化即改挂到人。
func (s *Store) UpdateGift(id int64, p GiftPatch) error {
	sets := []string{}
	args := []any{}
	if p.Direction != "" {
		if p.Direction != "in" && p.Direction != "out" {
			return errors.New("方向必须是 in 或 out")
		}
		sets = append(sets, "direction=?")
		args = append(args, p.Direction)
	}
	if p.Amount != "" {
		c, err := ParseMoney(p.Amount)
		if err != nil {
			return err
		}
		if c <= 0 {
			return errors.New("金额必须大于 0")
		}
		sets = append(sets, "amount_cents=?")
		args = append(args, c)
	}
	if p.PersonID > 0 {
		sets = append(sets, "person_id=?")
		args = append(args, p.PersonID)
	}
	if p.EventID > 0 {
		sets = append(sets, "event_id=?")
		args = append(args, p.EventID)
	} else if p.EventID == -1 {
		sets = append(sets, "event_id=NULL")
	}
	if p.Date != "" {
		sets = append(sets, "occurred_on=?")
		args = append(args, validDate(p.Date))
	}
	if p.Method != "" {
		sets = append(sets, "method=?")
		args = append(args, p.Method)
	}
	if p.Note != "" {
		sets = append(sets, "note=?")
		args = append(args, p.Note)
	}
	if p.Settle != "" {
		if p.Settle != "done" && p.Settle != "skip" && p.Settle != "open" {
			return errors.New("回礼状态不合法")
		}
		v := p.Settle
		if v == "open" {
			v = ""
		}
		sets = append(sets, "settle=?")
		args = append(args, v)
	}
	if p.Thanks != "" {
		if p.Thanks != "pending" && p.Thanks != "sent" && p.Thanks != "skipped" {
			return errors.New("感谢状态不合法")
		}
		sets = append(sets, "thanks_status=?")
		args = append(args, p.Thanks)
	}
	if len(sets) == 0 {
		return errors.New("没有要改的内容")
	}
	args = append(args, id)
	_, err := s.db.Exec(fmt.Sprintf(`UPDATE gift_record SET %s WHERE id=?`, strings.Join(sets, ",")), args...)
	return err
}

type BabyPatch struct {
	BabyID   int64
	Category string
	Amount   string
	Date     string
	Merchant string
	Note     string
	Qty      float64
	Unit     string
}

func (s *Store) UpdateBaby(id int64, p BabyPatch) error {
	sets := []string{}
	args := []any{}
	if p.BabyID > 0 {
		sets = append(sets, "baby_id=?")
		args = append(args, p.BabyID)
	}
	if p.Category != "" {
		sets = append(sets, "category=?")
		args = append(args, p.Category)
	}
	if p.Amount != "" {
		c, err := ParseMoney(p.Amount)
		if err != nil {
			return err
		}
		if c <= 0 {
			return errors.New("金额必须大于 0")
		}
		sets = append(sets, "amount_cents=?")
		args = append(args, c)
	}
	if p.Date != "" {
		sets = append(sets, "occurred_on=?")
		args = append(args, validDate(p.Date))
	}
	if p.Merchant != "" {
		sets = append(sets, "merchant=?")
		args = append(args, p.Merchant)
	}
	if p.Note != "" {
		sets = append(sets, "note=?")
		args = append(args, p.Note)
	}
	if p.Qty > 0 {
		sets = append(sets, "qty=?")
		args = append(args, p.Qty)
	}
	if p.Unit != "" {
		sets = append(sets, "unit=?")
		args = append(args, p.Unit)
	}
	if len(sets) == 0 {
		return errors.New("没有要改的内容")
	}
	args = append(args, id)
	_, err := s.db.Exec(fmt.Sprintf(`UPDATE baby_expense SET %s WHERE id=?`, strings.Join(sets, ",")), args...)
	return err
}

// BatchItem 批量录入里的一笔礼金。
type BatchItem struct {
	Name     string
	Amount   string
	Relation string
	Note     string
}

// BatchGifts 办酒席时一次性录入几十笔礼金。
func (s *Store) BatchGifts(eventID int64, eventName, date, method string, items []BatchItem, by int64) (int, error) {
	if len(items) == 0 {
		return 0, errors.New("没有要录入的记录")
	}
	if eventID <= 0 && strings.TrimSpace(eventName) != "" {
		if v, err := s.EnsureEvent(eventName); err == nil {
			eventID = v
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	// 关键：事务持有写锁期间，所有写入都必须走 tx。
	// 若在这里调用 s.EnsurePerson（用的是连接池里的另一个连接），会被本事务的写锁阻塞，
	// 5 秒 busy_timeout 后报 "database is locked"，导致后续记录被静默跳过。
	ensurePerson := func(name, relation string) (int64, error) {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO person(name, relation) VALUES (?,?)`, name, relation); err != nil {
			return 0, err
		}
		var id int64
		if err := tx.QueryRow(`SELECT id FROM person WHERE name=?`, name).Scan(&id); err != nil {
			return 0, err
		}
		return id, nil
	}
	n := 0
	for _, it := range items {
		name := strings.TrimSpace(it.Name)
		if name == "" {
			continue
		}
		c, err := ParseMoney(it.Amount)
		if err != nil || c <= 0 {
			continue
		}
		pid, err := ensurePerson(name, it.Relation)
		if err != nil {
			return n, err
		}
		var ev any
		if eventID > 0 {
			ev = eventID
		}
		if _, err := tx.Exec(`
			INSERT INTO gift_record(person_id, event_id, direction, amount_cents, occurred_on, method, note, created_by)
			VALUES (?,?,'in',?,?,?,?,?)`, pid, ev, c, validDate(date), method, it.Note, nullID(by)); err != nil {
			return n, err
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return n, err
	}
	return n, nil
}

/* ==================== 区间统计 ==================== */

type DayAmount struct {
	Date  string `json:"date"`
	Cents int64  `json:"amount_cents"`
	Count int    `json:"count"`
}

type RangeStats struct {
	Range        string      `json:"range"`
	From         string      `json:"from"`
	To           string      `json:"to"`
	PrevFrom     string      `json:"-"`
	PrevTo       string      `json:"-"`
	GiftRecv     int64       `json:"gift_recv"`
	GiftSent     int64       `json:"gift_sent"`
	GiftNet      int64       `json:"gift_net"`
	GiftCnt      int         `json:"gift_count"`
	Active       int         `json:"active_persons"`
	NewPpl       int         `json:"new_persons"`
	BabyTot      int64       `json:"baby_total"`
	BabyCnt      int         `json:"baby_count"`
	PrevGiftRecv int64       `json:"prev_gift_recv"`
	PrevGiftSent int64       `json:"prev_gift_sent"`
	PrevBabyTot  int64       `json:"prev_baby_total"`
	Categories   []CatSum    `json:"categories"`
	Days         []DayAmount `json:"days"`
	Months       []MonthSum  `json:"months"`
	TopPersons   []CatSum    `json:"top_persons"`
}

func rangeBounds(r string) (from, to, prevFrom, prevTo string) {
	now := time.Now()
	today := now.Format("2006-01-02")
	switch r {
	case "week":
		wd := int(now.Weekday())
		if wd == 0 {
			wd = 7
		}
		mon := now.AddDate(0, 0, -(wd - 1))
		from = mon.Format("2006-01-02")
		to = today
		prevTo = mon.AddDate(0, 0, -1).Format("2006-01-02")
		prevFrom = mon.AddDate(0, 0, -7).Format("2006-01-02")
	case "month":
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
		to = today
		p := now.AddDate(0, -1, 0)
		prevFrom = time.Date(p.Year(), p.Month(), 1, 0, 0, 0, 0, p.Location()).Format("2006-01-02")
		prevTo = time.Date(p.Year(), p.Month()+1, 0, 0, 0, 0, 0, p.Location()).Format("2006-01-02")
	case "quarter":
		q := (int(now.Month())-1)/3*3 + 1
		from = time.Date(now.Year(), time.Month(q), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
		to = today
		prevFrom = time.Date(now.Year(), time.Month(q)-3, 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
		prevTo = from
	case "year":
		from = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
		to = today
		prevFrom = time.Date(now.Year()-1, 1, 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
		prevTo = time.Date(now.Year()-1, 12, 31, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	default: // all
		from = "1970-01-01"
		to = today
		prevFrom, prevTo = from, from
	}
	if prevFrom == "" || prevTo == "" {
		prevFrom, prevTo = from, from
	}
	return
}

func (s *Store) Stats(r string) (RangeStats, error) {
	return s.StatsFor(r, 0)
}

func (s *Store) StatsFor(r string, babyID int64) (RangeStats, error) {
	var v RangeStats
	v.Range = r
	v.From, v.To, v.PrevFrom, v.PrevTo = rangeBounds(r)
	_ = v.PrevFrom

	if err := s.db.QueryRow(`
		SELECT COALESCE(SUM(CASE WHEN direction='in' THEN amount_cents END),0),
		       COALESCE(SUM(CASE WHEN direction='out' THEN amount_cents END),0),
		       COUNT(*)
		FROM gift_record WHERE occurred_on BETWEEN ? AND ?`, v.From, v.To).
		Scan(&v.GiftRecv, &v.GiftSent, &v.GiftCnt); err != nil {
		return v, err
	}
	v.GiftNet = v.GiftRecv - v.GiftSent

	s.db.QueryRow(`SELECT COUNT(DISTINCT person_id) FROM gift_record WHERE occurred_on BETWEEN ? AND ?`, v.From, v.To).Scan(&v.Active)
	s.db.QueryRow(`
		SELECT COUNT(*) FROM person p WHERE p.id IN (
		  SELECT person_id FROM gift_record WHERE occurred_on BETWEEN ? AND ?)
		  AND (SELECT MIN(occurred_on) FROM gift_record r WHERE r.person_id=p.id) >= ?`, v.From, v.To, v.From).Scan(&v.NewPpl)
	babyClause, babyArgs := "", []any{}
	if babyID > 0 {
		babyClause = ` AND baby_id=?`
		babyArgs = append(babyArgs, babyID)
	}
	s.db.QueryRow(`SELECT COALESCE(SUM(amount_cents),0), COUNT(*) FROM baby_expense WHERE occurred_on BETWEEN ? AND ?`+babyClause, append([]any{v.From, v.To}, babyArgs...)...).Scan(&v.BabyTot, &v.BabyCnt)
	s.db.QueryRow(`
		SELECT COALESCE(SUM(CASE WHEN direction='in' THEN amount_cents END),0),
		       COALESCE(SUM(CASE WHEN direction='out' THEN amount_cents END),0)
		FROM gift_record WHERE occurred_on BETWEEN ? AND ?`, v.PrevFrom, v.PrevTo).Scan(&v.PrevGiftRecv, &v.PrevGiftSent)
	s.db.QueryRow(`SELECT COALESCE(SUM(amount_cents),0) FROM baby_expense WHERE occurred_on BETWEEN ? AND ?`+babyClause, append([]any{v.PrevFrom, v.PrevTo}, babyArgs...)...).Scan(&v.PrevBabyTot)

	// 分类
	if rows, err := s.db.Query(`
		SELECT category, SUM(amount_cents), COUNT(*) FROM baby_expense
		WHERE occurred_on BETWEEN ? AND ?`+babyClause+` GROUP BY category ORDER BY 2 DESC`, append([]any{v.From, v.To}, babyArgs...)...); err == nil {
		defer rows.Close()
		for rows.Next() {
			var c CatSum
			if err := rows.Scan(&c.Category, &c.Cents, &c.Count); err == nil {
				v.Categories = append(v.Categories, c)
			}
		}
	}
	// 人情往来 Top（按收礼金额）
	if rows, err := s.db.Query(`
		SELECT p.name, SUM(r.amount_cents), COUNT(r.id) FROM gift_record r
		JOIN person p ON p.id=r.person_id
		WHERE r.occurred_on BETWEEN ? AND ? GROUP BY p.id ORDER BY 2 DESC LIMIT 6`, v.From, v.To); err == nil {
		defer rows.Close()
		for rows.Next() {
			var c CatSum
			if err := rows.Scan(&c.Category, &c.Cents, &c.Count); err == nil {
				v.TopPersons = append(v.TopPersons, c)
			}
		}
	}
	// 按天（用于日历热力）
	if rows, err := s.db.Query(`
		SELECT occurred_on, SUM(amount_cents), COUNT(*) FROM baby_expense
		WHERE occurred_on BETWEEN ? AND ?`+babyClause+` GROUP BY occurred_on
		UNION ALL
		SELECT occurred_on, SUM(amount_cents), COUNT(*) FROM gift_record
		WHERE occurred_on BETWEEN ? AND ? GROUP BY occurred_on`, append(append([]any{v.From, v.To}, babyArgs...), []any{v.From, v.To}...)...); err == nil {
		defer rows.Close()
		m := map[string]DayAmount{}
		for rows.Next() {
			var d DayAmount
			if err := rows.Scan(&d.Date, &d.Cents, &d.Count); err == nil {
				if e, ok := m[d.Date]; ok {
					e.Cents += d.Cents
					e.Count += d.Count
					m[d.Date] = e
				} else {
					m[d.Date] = d
				}
			}
		}
		for _, d := range m {
			v.Days = append(v.Days, d)
		}
		sortDays(v.Days)
	}
	// 按月
	if rows, err := s.db.Query(`
		SELECT strftime('%Y-%m', occurred_on) ym, SUM(amount_cents), COUNT(*) FROM baby_expense`+func() string {
		if babyID > 0 {
			return ` WHERE baby_id=?`
		}
		return ``
	}()+`
		GROUP BY ym ORDER BY ym DESC LIMIT 12`, func() []any {
		if babyID > 0 {
			return []any{babyID}
		}
		return nil
	}()...); err == nil {
		defer rows.Close()
		for rows.Next() {
			var m MonthSum
			if err := rows.Scan(&m.Month, &m.Cents, &m.Count); err == nil {
				v.Months = append(v.Months, m)
			}
		}
	}
	return v, nil
}

func sortDays(d []DayAmount) {
	for i := 1; i < len(d); i++ {
		for j := i; j > 0 && d[j].Date < d[j-1].Date; j-- {
			d[j], d[j-1] = d[j-1], d[j]
		}
	}
}

/* ==================== 条件筛选 ==================== */

type GiftFilter struct {
	From     string
	To       string
	PersonID int64
	EventID  int64
	Relation string
	Dir      string
	Min      int64
	Max      int64
	Keyword  string
	Limit    int
}

func (s *Store) FilterGifts(f GiftFilter) ([]Gift, error) {
	w := []string{"1=1"}
	a := []any{}
	if f.From != "" {
		w = append(w, "r.occurred_on >= ?")
		a = append(a, f.From)
	}
	if f.To != "" {
		w = append(w, "r.occurred_on <= ?")
		a = append(a, f.To)
	}
	if f.PersonID > 0 {
		w = append(w, "r.person_id = ?")
		a = append(a, f.PersonID)
	}
	if f.EventID > 0 {
		w = append(w, "r.event_id = ?")
		a = append(a, f.EventID)
	}
	if f.Relation != "" {
		w = append(w, "COALESCE(p.relation,'') = ?")
		a = append(a, f.Relation)
	}
	if f.Dir != "" {
		w = append(w, "r.direction = ?")
		a = append(a, f.Dir)
	}
	if f.Min > 0 {
		w = append(w, "r.amount_cents >= ?")
		a = append(a, f.Min)
	}
	if f.Max > 0 {
		w = append(w, "r.amount_cents <= ?")
		a = append(a, f.Max)
	}
	if f.Keyword != "" {
		kw := "%" + f.Keyword + "%"
		w = append(w, "(p.name LIKE ? OR COALESCE(e.name,'') LIKE ? OR COALESCE(r.note,'') LIKE ? OR COALESCE(r.method,'') LIKE ?)")
		a = append(a, kw, kw, kw, kw)
	}
	lim := f.Limit
	if lim <= 0 {
		lim = 200
	}
	base := `
	SELECT r.id, r.person_id, p.name, COALESCE(r.event_id,0), COALESCE(e.name,''),
	       r.direction, r.amount_cents, r.occurred_on, COALESCE(r.method,''), COALESCE(r.note,''),
	       COALESCE(r.settle,''), COALESCE(r.thanks_status,'pending'), COALESCE(m.nickname,'')
	FROM gift_record r
	JOIN person p ON p.id = r.person_id
	LEFT JOIN gift_event e ON e.id = r.event_id
	LEFT JOIN member m ON m.id = r.created_by
	WHERE ` + strings.Join(w, " AND ") + ` ORDER BY r.occurred_on DESC, r.id DESC LIMIT ?`
	a = append(a, lim)
	rows, err := s.db.Query(base, a...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Gift
	for rows.Next() {
		var g Gift
		if err := rows.Scan(&g.ID, &g.PersonID, &g.Person, &g.EventID, &g.Event, &g.Direction, &g.Cents, &g.On, &g.Method, &g.Note, &g.Settle, &g.Thanks, &g.CreatedBy); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

/* ==================== 完整数据导出 / 恢复 ==================== */

type Snapshot struct {
	Version      string             `json:"version"`
	ExportedAt   string             `json:"exported_at"`
	Persons      []Person           `json:"persons"`
	Events       []Event            `json:"events"`
	Gifts        []Gift             `json:"gifts"`
	Babies       []BabyExp          `json:"babies"`
	Milestones   []Milestone        `json:"milestones"`
	Members      []Member           `json:"members"`
	BabyProfiles []BabyProfile      `json:"baby_profiles"`
	Health       []BackupHealth     `json:"health_records"`
	Reminders    []BackupReminder   `json:"reminders"`
	Tasks        []BackupTask       `json:"tasks"`
	Shopping     []BackupShopping   `json:"shopping"`
	Attachments  []BackupAttachment `json:"attachments"`
	Audit        []BackupAudit      `json:"audit_log"`
}

func (s *Store) ExportAll() (*Snapshot, error) {
	snap := &Snapshot{Version: "1", ExportedAt: time.Now().Format(timeLayout)}
	var err error
	if snap.Persons, err = s.SearchPersons("", 100000); err != nil {
		return nil, err
	}
	if snap.Events, err = s.ListEvents(); err != nil {
		return nil, err
	}
	if snap.Gifts, err = s.ListGifts(0, 1000000); err != nil {
		return nil, err
	}
	if snap.Babies, err = s.ListBaby(1000000); err != nil {
		return nil, err
	}
	if snap.Milestones, err = s.ListMilestones(100000); err != nil {
		return nil, err
	}
	if snap.Members, err = s.ListMembers(); err != nil {
		return nil, err
	}
	if snap.BabyProfiles, err = s.ListBabies(); err != nil {
		return nil, err
	}
	if snap.Health, err = listBackupRows[BackupHealth](s.db, `SELECT id,baby_id,kind,title,occurred_on,COALESCE(value,''),COALESCE(note,''),COALESCE(created_by,0),created_at FROM health_record ORDER BY id`, func(r *sql.Rows, v *BackupHealth) error {
		return r.Scan(&v.ID, &v.BabyID, &v.Kind, &v.Title, &v.On, &v.Value, &v.Note, &v.CreatedBy, &v.CreatedAt)
	}); err != nil {
		return nil, err
	}
	if snap.Reminders, err = listBackupRows[BackupReminder](s.db, `SELECT id,baby_id,kind,title,due_on,repeat_days,status,COALESCE(note,''),COALESCE(created_by,0),created_at FROM reminder ORDER BY id`, func(r *sql.Rows, v *BackupReminder) error {
		return r.Scan(&v.ID, &v.BabyID, &v.Kind, &v.Title, &v.DueOn, &v.RepeatDays, &v.Status, &v.Note, &v.CreatedBy, &v.CreatedAt)
	}); err != nil {
		return nil, err
	}
	if snap.Tasks, err = listBackupRows[BackupTask](s.db, `SELECT id,title,COALESCE(note,''),kind,COALESCE(due_on,''),COALESCE(assignee_id,0),status,COALESCE(created_by,0),created_at,updated_at FROM family_task ORDER BY id`, func(r *sql.Rows, v *BackupTask) error {
		return r.Scan(&v.ID, &v.Title, &v.Note, &v.Kind, &v.DueOn, &v.AssigneeID, &v.Status, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt)
	}); err != nil {
		return nil, err
	}
	if snap.Shopping, err = listBackupRows[BackupShopping](s.db, `SELECT id,name,COALESCE(category,''),COALESCE(qty,0),COALESCE(unit,''),checked,COALESCE(bought_by,0),COALESCE(note,''),COALESCE(created_by,0),created_at,updated_at FROM shopping_item ORDER BY id`, func(r *sql.Rows, v *BackupShopping) error {
		return r.Scan(&v.ID, &v.Name, &v.Category, &v.Qty, &v.Unit, &v.Checked, &v.BoughtBy, &v.Note, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt)
	}); err != nil {
		return nil, err
	}
	if snap.Attachments, err = listBackupRows[BackupAttachment](s.db, `SELECT id,kind,record_id,file_name,mime,data,COALESCE(created_by,0),created_at FROM attachment ORDER BY id`, func(r *sql.Rows, v *BackupAttachment) error {
		return r.Scan(&v.ID, &v.Kind, &v.RecordID, &v.FileName, &v.Mime, &v.Data, &v.CreatedBy, &v.CreatedAt)
	}); err != nil {
		return nil, err
	}
	if snap.Audit, err = listBackupRows[BackupAudit](s.db, `SELECT id,action,entity,COALESCE(entity_id,0),COALESCE(detail,''),COALESCE(member_id,0),created_at FROM audit_log ORDER BY id`, func(r *sql.Rows, v *BackupAudit) error {
		return r.Scan(&v.ID, &v.Action, &v.Entity, &v.EntityID, &v.Detail, &v.MemberID, &v.CreatedAt)
	}); err != nil {
		return nil, err
	}
	return snap, nil
}

func listBackupRows[T any](db *sql.DB, query string, scan func(*sql.Rows, *T) error) ([]T, error) {
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		var v T
		if err := scan(rows, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ImportAll 把快照写回库（同一姓名/事由会复用，不会重复建人）。
func (s *Store) ImportAll(snap *Snapshot) (int, error) {
	if snap == nil {
		return 0, errors.New("文件内容为空")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	// 同 BatchGifts：事务期间一律走 tx，不能调用 s.EnsurePerson / s.EnsureEvent
	ensurePerson := func(name, relation string) (int64, error) {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO person(name, relation) VALUES (?,?)`, name, relation); err != nil {
			return 0, err
		}
		var id int64
		if err := tx.QueryRow(`SELECT id FROM person WHERE name=?`, name).Scan(&id); err != nil {
			return 0, err
		}
		return id, nil
	}
	ensureEvent := func(name string) (int64, error) {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO gift_event(name) VALUES (?)`, name); err != nil {
			return 0, err
		}
		var id int64
		if err := tx.QueryRow(`SELECT id FROM gift_event WHERE name=?`, name).Scan(&id); err != nil {
			return 0, err
		}
		return id, nil
	}
	n := 0
	for _, m := range snap.Members {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO member(id,nickname,is_active) VALUES(?,?,1)`, m.ID, m.Nickname); err != nil {
			return n, err
		}
	}
	for _, b := range snap.BabyProfiles {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO baby_profile(id,name,birthday,note,is_active) VALUES(?,?,?,?,?)`, b.ID, b.Name, b.Birthday, b.Note, boolInt(b.Active)); err != nil {
			return n, err
		}
	}
	for _, p := range snap.Persons {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO person(name, relation, py, note) VALUES (?,?,?,?)`, p.Name, p.Relation, p.Py, p.Note); err != nil {
			return n, err
		}
	}
	for _, e := range snap.Events {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO gift_event(name, event_date, note,status,venue,budget_cents,guest_count) VALUES (?,?,?,?,?,?,?)`, e.Name, e.Date, e.Note, e.Status, e.Venue, e.BudgetCents, e.GuestCount); err != nil {
			return n, err
		}
		_, _ = tx.Exec(`UPDATE gift_event SET event_date=?,note=?,status=?,venue=?,budget_cents=?,guest_count=? WHERE name=?`, e.Date, e.Note, e.Status, e.Venue, e.BudgetCents, e.GuestCount, e.Name)
	}
	for _, g := range snap.Gifts {
		pid, err := ensurePerson(g.Person, "")
		if err != nil {
			return n, err
		}
		var ev any
		if strings.TrimSpace(g.Event) != "" {
			if id, err := ensureEvent(g.Event); err == nil {
				ev = id
			}
		}
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO gift_record(id,person_id, event_id, direction, amount_cents, occurred_on, method, note,settle,thanks_status)
			VALUES (?,?,?,?,?,?,?,?,?,?)`, g.ID, pid, ev, g.Direction, g.Cents, g.On, g.Method, g.Note, g.Settle, g.Thanks); err != nil {
			continue
		}
		n++
	}
	for _, b := range snap.Babies {
		var q any
		if b.Qty > 0 {
			q = b.Qty
		}
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO baby_expense(id,baby_id,category, amount_cents, occurred_on, merchant, note, qty, unit)
			VALUES (?,?,?,?,?,?,?,?,?)`, b.ID, max(int64(1), b.BabyID), b.Category, b.Cents, b.On, b.Merchant, b.Note, q, b.Unit); err != nil {
			continue
		}
		n++
	}
	for _, m := range snap.Milestones {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO milestone(id,baby_id,kind,title,occurred_on,note) VALUES (?,?,?,?,?,?)`, m.ID, max(int64(1), m.BabyID), m.Kind, m.Title, m.On, m.Note); err != nil {
			continue
		}
		n++
	}
	for _, h := range snap.Health {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO health_record(id,baby_id,kind,title,occurred_on,value,note,created_by,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, h.ID, max(int64(1), h.BabyID), h.Kind, h.Title, h.On, h.Value, h.Note, nullID(h.CreatedBy), h.CreatedAt); err != nil {
			return n, err
		}
		n++
	}
	for _, x := range snap.Reminders {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO reminder(id,baby_id,kind,title,due_on,repeat_days,status,note,created_by,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, x.ID, max(int64(1), x.BabyID), x.Kind, x.Title, x.DueOn, x.RepeatDays, x.Status, x.Note, nullID(x.CreatedBy), x.CreatedAt); err != nil {
			return n, err
		}
		n++
	}
	for _, x := range snap.Tasks {
		var due any
		if x.DueOn != "" {
			due = x.DueOn
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO family_task(id,title,note,kind,due_on,assignee_id,status,created_by,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, x.ID, x.Title, x.Note, x.Kind, due, nullID(x.AssigneeID), x.Status, nullID(x.CreatedBy), x.CreatedAt, x.UpdatedAt); err != nil {
			return n, err
		}
		n++
	}
	for _, x := range snap.Shopping {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO shopping_item(id,name,category,qty,unit,checked,bought_by,note,created_by,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, x.ID, x.Name, x.Category, x.Qty, x.Unit, x.Checked, nullID(x.BoughtBy), x.Note, nullID(x.CreatedBy), x.CreatedAt, x.UpdatedAt); err != nil {
			return n, err
		}
		n++
	}
	for _, x := range snap.Attachments {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO attachment(id,kind,record_id,file_name,mime,data,created_by,created_at) VALUES(?,?,?,?,?,?,?,?)`, x.ID, x.Kind, x.RecordID, x.FileName, x.Mime, x.Data, nullID(x.CreatedBy), x.CreatedAt); err != nil {
			return n, err
		}
		n++
	}
	for _, x := range snap.Audit {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO audit_log(id,action,entity,entity_id,detail,member_id,created_at) VALUES(?,?,?,?,?,?,?)`, x.ID, x.Action, x.Entity, nullID(x.EntityID), x.Detail, nullID(x.MemberID), x.CreatedAt); err != nil {
			return n, err
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return n, err
	}
	return n, nil
}

// ---- 备份 ----

// VacuumInto 生成不停服的一致性快照。
func (s *Store) VacuumInto(file string) error {
	_, err := s.db.Exec(fmt.Sprintf(`VACUUM INTO %q`, file))
	return err
}

func nullID(i int64) any {
	if i <= 0 {
		return nil
	}
	return i
}
