package main

import (
	"errors"
	"strings"
	"time"
)

type HealthRecord struct {
	ID        int64  `json:"id"`
	BabyID    int64  `json:"baby_id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	On        string `json:"occurred_on"`
	Value     string `json:"value,omitempty"`
	Note      string `json:"note,omitempty"`
	CreatedBy string `json:"created_by,omitempty"`
}

func (s *Store) ListHealth(babyID int64, limit int) ([]HealthRecord, error) {
	if babyID <= 0 {
		babyID = 1
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.Query(`
		SELECT h.id,h.baby_id,h.kind,h.title,h.occurred_on,COALESCE(h.value,''),COALESCE(h.note,''),COALESCE(m.nickname,'')
		FROM health_record h LEFT JOIN member m ON m.id=h.created_by
		WHERE h.baby_id=? ORDER BY h.occurred_on DESC,h.id DESC LIMIT ?`, babyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HealthRecord
	for rows.Next() {
		var v HealthRecord
		if err := rows.Scan(&v.ID, &v.BabyID, &v.Kind, &v.Title, &v.On, &v.Value, &v.Note, &v.CreatedBy); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) AddHealth(babyID int64, kind, title, date, value, note string, by int64) (int64, error) {
	if babyID <= 0 {
		babyID = 1
	}
	if strings.TrimSpace(kind) == "" {
		kind = "成长健康"
	}
	if strings.TrimSpace(title) == "" {
		return 0, errors.New("请填写健康记录标题")
	}
	res, err := s.db.Exec(`INSERT INTO health_record(baby_id,kind,title,occurred_on,value,note,created_by) VALUES(?,?,?,?,?,?,?)`,
		babyID, strings.TrimSpace(kind), strings.TrimSpace(title), validDate(date), strings.TrimSpace(value), strings.TrimSpace(note), nullID(by))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) DeleteHealth(id int64) error {
	_, err := s.db.Exec(`DELETE FROM health_record WHERE id=?`, id)
	return err
}

type Reminder struct {
	ID         int64  `json:"id"`
	BabyID     int64  `json:"baby_id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	DueOn      string `json:"due_on"`
	RepeatDays int    `json:"repeat_days"`
	Status     string `json:"status"`
	Note       string `json:"note,omitempty"`
}

func (s *Store) ListReminders(babyID int64, status string, limit int) ([]Reminder, error) {
	if babyID <= 0 {
		babyID = 1
	}
	if limit <= 0 {
		limit = 200
	}
	where := "WHERE r.baby_id=?"
	args := []any{babyID}
	if status != "" {
		where += " AND r.status=?"
		args = append(args, status)
	}
	args = append(args, limit)
	rows, err := s.db.Query(`SELECT r.id,r.baby_id,r.kind,r.title,r.due_on,r.repeat_days,r.status,COALESCE(r.note,'') FROM reminder r `+where+` ORDER BY r.status,r.due_on,r.id LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Reminder
	for rows.Next() {
		var v Reminder
		if err := rows.Scan(&v.ID, &v.BabyID, &v.Kind, &v.Title, &v.DueOn, &v.RepeatDays, &v.Status, &v.Note); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) AddReminder(babyID int64, kind, title, due string, repeat int, note string, by int64) (int64, error) {
	if babyID <= 0 {
		babyID = 1
	}
	if strings.TrimSpace(title) == "" || strings.TrimSpace(due) == "" {
		return 0, errors.New("提醒内容和日期不能为空")
	}
	if repeat < 0 {
		repeat = 0
	}
	res, err := s.db.Exec(`INSERT INTO reminder(baby_id,kind,title,due_on,repeat_days,note,created_by) VALUES(?,?,?,?,?,?,?)`,
		babyID, strings.TrimSpace(kind), strings.TrimSpace(title), validDate(due), repeat, strings.TrimSpace(note), nullID(by))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateReminder(id int64, status string) error {
	if status != "open" && status != "done" {
		return errors.New("提醒状态不合法")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldStatus string
	var babyID int64
	var kind, title, due, note string
	var repeat int
	if err := tx.QueryRow(`SELECT baby_id,kind,title,due_on,repeat_days,status,COALESCE(note,'') FROM reminder WHERE id=?`, id).
		Scan(&babyID, &kind, &title, &due, &repeat, &oldStatus, &note); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE reminder SET status=? WHERE id=?`, status, id); err != nil {
		return err
	}
	// 只有首次完成时才创建下一次，重复提交不会产生重复提醒。
	if status == "done" && oldStatus != "done" && repeat > 0 {
		// 逾期完成的下一次不能再落在过去：基准取「原到期日」和「今天」里较晚的那个，
		// 否则拖了半年的体检做完，下一次补出来还是半年前的日期。
		today := time.Now()
		base, err := time.Parse("2006-01-02", due)
		if err != nil || base.Before(today) {
			base = today
		}
		next := base.AddDate(0, 0, repeat).Format("2006-01-02")
		if _, err := tx.Exec(`INSERT INTO reminder(baby_id,kind,title,due_on,repeat_days,note,created_by) SELECT ?,?,?,?,?,?,created_by FROM reminder WHERE id=?`, babyID, kind, title, next, repeat, note, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteReminder(id int64) error {
	_, err := s.db.Exec(`DELETE FROM reminder WHERE id=?`, id)
	return err
}
