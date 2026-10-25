package main

import (
	"math"
	"strings"
	"time"
)

// 本文件承载「洞察」类能力：消耗品预测、人情紧迫度、成长时间线、成本预测。
// 这些是普通记账软件做不出来的部分。

// 消耗品分类：有明确消耗周期、需要补货预警的
var consumableCats = map[string]string{
	"奶粉": "罐", "尿不湿": "包", "辅食": "盒", "洗护": "瓶", "纸巾湿巾": "包",
}

func IsConsumable(cat string) bool {
	_, ok := consumableCats[cat]
	return ok
}

func defaultUnit(cat string) string {
	if u, ok := consumableCats[cat]; ok {
		return u
	}
	return ""
}

/* ==================== 消耗品补货预测 ==================== */

type Consumable struct {
	Category   string  `json:"category"`
	Unit       string  `json:"unit"`
	DailyUse   float64 `json:"daily_use"`  // 日均消耗量
	Stock      float64 `json:"stock"`      // 估算剩余库存
	DaysLeft   int     `json:"days_left"`  // 预计还能撑多少天
	RunOutOn   string  `json:"run_out_on"` // 预计用完日期
	LastQty    float64 `json:"last_qty"`
	LastOn     string  `json:"last_on"`
	BuyCount   int     `json:"buy_count"`
	AvgPrice   int64   `json:"avg_price"` // 平均单价（分）
	TotalCost  int64   `json:"total_cost"`
	Confidence string  `json:"confidence"` // high/mid/low
}

// consumePrediction 预测每个消耗品的剩余可用天数。
// 算法：以相邻两次购买之间的消耗反推日均用量，再除以最后一次购买量得到剩余天数。
func (s *Store) Consumables() ([]Consumable, error) {
	return s.ConsumablesFor(0)
}

func (s *Store) ConsumablesFor(babyID int64) ([]Consumable, error) {
	var out []Consumable
	for cat := range consumableCats {
		query := `
			SELECT occurred_on, qty, amount_cents FROM baby_expense
			WHERE category=? AND qty IS NOT NULL AND qty>0
			  AND julianday('now','localtime') - julianday(occurred_on) <= 180`
		args := []any{cat}
		if babyID > 0 { query += ` AND baby_id=?`; args = append(args, babyID) }
		query += ` ORDER BY occurred_on`
		rows, err := s.db.Query(query, args...)
		if err != nil {
			return nil, err
		}
		type item struct {
			on  string
			qty float64
			amt int64
		}
		var list []item
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.on, &it.qty, &it.amt); err != nil {
				rows.Close()
				return nil, err
			}
			list = append(list, it)
		}
		rows.Close()

		if len(list) < 2 {
			continue
		}
		first, _ := time.Parse("2006-01-02", list[0].on)
		last, _ := time.Parse("2006-01-02", list[len(list)-1].on)
		spanDays := last.Sub(first).Hours() / 24
		if spanDays < 1 {
			continue
		}
		var totalQty, totalAmt float64
		for _, it := range list {
			totalQty += it.qty
			totalAmt += float64(it.amt)
		}
		lastQty := list[len(list)-1].qty
		// 期间消耗量 = 总量 - 最后一次仍在家里的量
		consumed := totalQty - lastQty
		if consumed <= 0 {
			continue
		}
		daily := consumed / spanDays
		if daily <= 0 {
			continue
		}
		daysSinceLast := time.Since(last).Hours() / 24
		stock := lastQty - daily*daysSinceLast
		if stock < 0 {
			stock = 0
		}
		daysLeft := int(math.Floor(stock / daily))
		runOut := last.AddDate(0, 0, int(math.Ceil(lastQty/daily)))

		c := Consumable{
			Category:  cat,
			Unit:      consumableCats[cat],
			DailyUse:  mathRound2(daily),
			Stock:     mathRound2(stock),
			DaysLeft:  daysLeft,
			RunOutOn:  runOut.Format("2006-01-02"),
			LastQty:   lastQty,
			LastOn:    list[len(list)-1].on,
			BuyCount:  len(list),
			AvgPrice:  int64(totalAmt / totalQty),
			TotalCost: int64(totalAmt),
		}
		switch {
		case len(list) >= 4 && spanDays >= 30:
			c.Confidence = "high"
		case len(list) >= 3:
			c.Confidence = "mid"
		default:
			c.Confidence = "low"
		}
		out = append(out, c)
	}
	sortConsumables(out)
	return out, nil
}

func mathRound2(v float64) float64 { return math.Round(v*100) / 100 }

func sortConsumables(cs []Consumable) {
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0 && cs[j].DaysLeft < cs[j-1].DaysLeft; j-- {
			cs[j], cs[j-1] = cs[j-1], cs[j]
		}
	}
}

/* ==================== 成长里程碑 ==================== */

type Milestone struct {
	ID     int64  `json:"id"`
	BabyID int64  `json:"baby_id"`
	Kind   string `json:"kind"`
	Title  string `json:"title"`
	On     string `json:"occurred_on"`
	Note   string `json:"note,omitempty"`
	By     string `json:"created_by,omitempty"`
}

var defaultMilestones = []struct{ kind, title string }{
	{"出生", "出生啦"},
	{"满月", "满月"},
	{"百天", "百日"},
	{"周岁", "周岁"},
	{"疫苗", "疫苗接种"},
	{"体检", "儿保体检"},
	{"翻身", "第一次翻身"},
	{"坐", "会坐了"},
	{"爬", "会爬了"},
	{"站", "第一次站立"},
	{"走", "迈出第一步"},
	{"牙", "长出第一颗牙"},
	{"说话", "第一次叫爸爸妈妈"},
	{"入园", "上幼儿园"},
	{"就医", "生病就医"},
}

func (s *Store) AddMilestone(kind, title, date, note string, by int64) (int64, error) {
	return s.AddMilestoneFor(1, kind, title, date, note, by)
}

func (s *Store) AddMilestoneFor(babyID int64, kind, title, date, note string, by int64) (int64, error) {
	if strings.TrimSpace(title) == "" {
		title = kind
	}
	if babyID <= 0 {
		babyID = 1
	}
	res, err := s.db.Exec(`INSERT INTO milestone(baby_id,kind,title,occurred_on,note,created_by) VALUES(?,?,?,?,?,?)`,
		babyID, kind, strings.TrimSpace(title), validDate(date), note, nullID(by))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) DeleteMilestone(id int64) error {
	_, err := s.db.Exec(`DELETE FROM milestone WHERE id=?`, id)
	return err
}

func (s *Store) ListMilestones(limit int) ([]Milestone, error) {
	return s.ListMilestonesFor(0, limit)
}

func (s *Store) ListMilestonesFor(babyID int64, limit int) ([]Milestone, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `
		SELECT m.id, COALESCE(m.baby_id,1), m.kind, m.title, m.occurred_on, COALESCE(m.note,''), COALESCE(u.nickname,'')
		FROM milestone m LEFT JOIN member u ON u.id=m.created_by
	`
	args := []any{}
	if babyID > 0 {
		query += ` WHERE m.baby_id=?`
		args = append(args, babyID)
	}
	query += ` ORDER BY m.occurred_on DESC, m.id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Milestone
	for rows.Next() {
		var m Milestone
		if err := rows.Scan(&m.ID, &m.BabyID, &m.Kind, &m.Title, &m.On, &m.Note, &m.By); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

/* ==================== 混合时间线 ==================== */
// 把「花了什么钱」「收了谁的礼」「宝宝长大了什么」揉进同一条时间轴。

type TLItem struct {
	Type  string `json:"type"` // gift | baby | milestone
	On    string `json:"date"`
	Title string `json:"title"`
	Sub   string `json:"sub,omitempty"`
	Cents int64  `json:"cents,omitempty"`
	Dir   string `json:"dir,omitempty"` // gift: in/out ；baby: out
	ID    int64  `json:"id"`
	Badge string `json:"badge,omitempty"`
}

func (s *Store) Timeline(limit int) ([]TLItem, error) {
	return s.TimelineFor(0, limit)
}

func (s *Store) TimelineFor(babyID int64, limit int) ([]TLItem, error) {
	if limit <= 0 {
		limit = 120
	}
	var out []TLItem
	// 人情
	if gifts, err := s.ListGifts(0, 200); err == nil {
		for _, g := range gifts {
			t := g.Person
			if g.Event != "" {
				t = g.Person + " · " + g.Event
			}
			out = append(out, TLItem{Type: "gift", On: g.On, Title: t,
				Sub: g.Note, Cents: g.Cents, Dir: g.Direction, ID: g.ID,
				Badge: map[string]string{"in": "收到", "out": "送出"}[g.Direction]})
		}
	}
	// 宝宝开销
	if babies, err := s.ListBabyFor(babyID, 200); err == nil {
		for _, b := range babies {
			out = append(out, TLItem{Type: "baby", On: b.On, Title: b.Category, Sub: b.Note,
				Cents: b.Cents, Dir: "out", ID: b.ID, Badge: "支出"})
		}
	}
	// 里程碑
	if ms, err := s.ListMilestonesFor(babyID, 100); err == nil {
		for _, m := range ms {
			out = append(out, TLItem{Type: "milestone", On: m.On, Title: m.Title, Sub: m.Note, ID: m.ID, Badge: m.Kind})
		}
	}
	sortTL(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func sortTL(a []TLItem) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j].On > a[j-1].On; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

/* ==================== 首页洞察 ==================== */

type Alert struct {
	Level    string `json:"level"` // urgent | warn | info
	Title    string `json:"title"`
	Desc     string `json:"desc"`
	Kind     string `json:"kind"` // owe | supply | quiet
	Amount   int64  `json:"amount,omitempty"`
	PersonID int64  `json:"person_id,omitempty"`
}

type Insights struct {
	MonthCents     int64        `json:"month_cents"`
	LastMonthCents int64        `json:"last_month_cents"`
	MonthDelta     int          `json:"month_delta"` // 百分比
	MonthGiftNet   int64        `json:"month_gift_net"`
	BabyTotal      int64        `json:"baby_total"`
	GiftSummary    GiftSummary  `json:"gift"`
	Alerts         []Alert      `json:"alerts"`
	Consumables    []Consumable `json:"consumables"`
	ProjectCents   int64        `json:"project_year_cents"`
	ForecastTo3Y   int64        `json:"forecast_to_3y"`
	BabyMonths     int          `json:"baby_months"`
	UpcomingMS     []string     `json:"upcoming_ms"`
}

func (s *Store) Insights() (Insights, error) {
	return s.InsightsFor(0)
}

func (s *Store) InsightsFor(babyID int64) (Insights, error) {
	var v Insights
	curPrefix := time.Now().Format("2006-01")
	lastPrefix := time.Now().AddDate(0, -1, 1).Format("2006-01")
	babyClause, babyArgs := "", []any{}
	if babyID > 0 { babyClause = ` AND baby_id=?`; babyArgs = []any{babyID} }

	if err := s.db.QueryRow(`SELECT COALESCE(SUM(amount_cents),0) FROM baby_expense WHERE strftime('%Y-%m',occurred_on)=?`+babyClause, append([]any{curPrefix}, babyArgs...)...).Scan(&v.MonthCents); err != nil {
		return v, err
	}
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(amount_cents),0) FROM baby_expense WHERE strftime('%Y-%m',occurred_on)=?`+babyClause, append([]any{lastPrefix}, babyArgs...)...).Scan(&v.LastMonthCents); err != nil {
		return v, err
	}
	if v.LastMonthCents > 0 {
		v.MonthDelta = int(float64(v.MonthCents-v.LastMonthCents) / float64(v.LastMonthCents) * 100)
	}
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(amount_cents),0) FROM baby_expense WHERE 1=1`+babyClause, babyArgs...).Scan(&v.BabyTotal); err != nil {
		return v, err
	}
	if gs, err := s.GiftSummary(); err != nil {
		return v, err
	} else {
		v.GiftSummary = gs
	}

	// 消耗品预警
	if cs, err := s.ConsumablesFor(babyID); err == nil {
		v.Consumables = cs
		for _, c := range cs {
			if c.DaysLeft <= 10 {
				lvl := "warn"
				if c.DaysLeft <= 3 {
					lvl = "urgent"
				}
				v.Alerts = append(v.Alerts, Alert{
					Level: lvl, Kind: "supply", Amount: c.AvgPrice,
					Title: c.Category + "快用完了",
					Desc:  "按日均 " + fmtF(c.DailyUse) + " " + c.Unit + "，约 " + itoa(c.DaysLeft) + " 天后见底",
				})
			}
		}
	}

	// 待回礼紧迫度：拖得越久越失礼
	if owes, err := s.OweList(200); err == nil {
		for _, p := range owes {
			// 紧迫度按「最后一次收到对方礼金」算，而不是最后一次往来——
			// 昨天刚回过礼的人不该和两年前收了钱一分没还的人同等对待。
			base := p.LastInOn
			if base == "" {
				base = p.LastOn
			}
			days := daysSince(base)
			if days < 0 {
				continue
			}
			level := "info"
			hint := ""
			switch {
			case days >= 365:
				level = "urgent"
				hint = "拖了一年多"
			case days >= 120:
				level = "warn"
				hint = "有些久了"
			case days >= 60:
				level = "info"
				hint = "可以安排了"
			}
			if hint == "" {
				continue
			}
			v.Alerts = append(v.Alerts, Alert{
				Level: level, Kind: "owe", Amount: p.NetCents, PersonID: p.ID,
				Title: "还欠 " + p.Name + " " + fmtYuan(p.NetCents) + " 元",
				Desc: hint + " · 距今 " + itoa(days) + " 天 · 累计收 " + fmtYuan(p.RecvCents) +
					" / 送 " + fmtYuan(p.SentCents),
			})
		}
	}

	// 久未往来：关系会凉
	rows, err := s.db.Query(personSelect + `
		WHERE (SELECT MAX(occurred_on) FROM gift_record r WHERE r.person_id=p.id) IS NOT NULL
		  AND julianday('now','localtime') - julianday((SELECT MAX(occurred_on) FROM gift_record r WHERE r.person_id=p.id)) > 730`)
	if err == nil {
		defer rows.Close()
		var quiet int
		for rows.Next() {
			p, err := scanPerson(rows)
			if err != nil {
				continue
			}
			quiet++
			if quiet <= 2 {
				v.Alerts = append(v.Alerts, Alert{
					Level: "info", Kind: "quiet", PersonID: p.ID,
					Title: "和 " + p.Name + " 很久没走动了",
					Desc:  "上次往来是 " + p.LastOn,
				})
			}
		}
		if quiet > 2 {
			v.Alerts = append(v.Alerts, Alert{
				Level: "info", Kind: "quiet",
				Title: "还有 " + itoa(quiet-2) + " 位亲友超过两年没往来",
				Desc:  "人情是走动出来的",
			})
		}
	}

	// 近三个月均值 → 预测未来一年；有生日时顺推到 3 岁
	var recent int64
	var monthsCnt int
	err = s.db.QueryRow(`
		SELECT COALESCE(SUM(amount_cents),0), COUNT(DISTINCT strftime('%Y-%m',occurred_on))
		FROM baby_expense WHERE julianday('now','localtime') - julianday(occurred_on) <= 92`).Scan(&recent, &monthsCnt)
	if err != nil {
		return v, err
	}
	birthday := s.GetProfile("baby_birthday")
	if babyID > 0 { _ = s.db.QueryRow(`SELECT COALESCE(birthday,'') FROM baby_profile WHERE id=?`, babyID).Scan(&birthday) }
	if birthday != "" {
		if bt, perr := time.Parse("2006-01-02", birthday); perr == nil {
			v.BabyMonths = int(time.Since(bt).Hours() / 24 / 30.44)
			if v.BabyMonths < 0 {
				v.BabyMonths = 0
			}
		}
	}
	if monthsCnt > 0 {
		avg := recent / int64(monthsCnt)
		v.ProjectCents = avg * 12
		if m := 36 - v.BabyMonths; m > 0 {
			v.ForecastTo3Y = v.BabyTotal + avg*int64(m)
		}
	}

	sortAlerts(v.Alerts)
	return v, nil
}

func sortAlerts(as []Alert) {
	rank := map[string]int{"urgent": 0, "warn": 1, "info": 2}
	for i := 1; i < len(as); i++ {
		for j := i; j > 0 && rank[as[j].Level] < rank[as[j-1].Level]; j-- {
			as[j], as[j-1] = as[j-1], as[j]
		}
	}
}

func daysSince(d string) int {
	if d == "" {
		return -1
	}
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return -1
	}
	return int(time.Since(t).Hours() / 24)
}

func itoa(n int) string { return intToStr(n) }

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func fmtF(v float64) string {
	s := strings.TrimRight(strings.TrimRight(strF(v, 2), "0"), ".")
	if s == "" {
		s = "0"
	}
	return s
}

func strF(v float64, n int) string {
	p := 1.0
	for i := 0; i < n; i++ {
		p *= 10
	}
	r := math.Round(v*p) / p
	i := int64(r)
	f := int64(math.Round((r - float64(i)) * p))
	if f < 0 {
		f = -f
	}
	if n == 0 {
		return intToStr(int(i))
	}
	return intToStr(int(i)) + "." + padLeft(intToStr(int(f)), n)
}

func padLeft(s string, n int) string {
	for len(s) < n {
		s = "0" + s
	}
	return s
}

func fmtYuan(cents int64) string {
	return strF(float64(cents)/100, 2)
}
