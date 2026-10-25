package main

import (
	"encoding/base64"
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func memberID(r *http.Request) int64 {
	if m := currentMember(r); m != nil {
		return m.ID
	}
	return 0
}

func handleGetEvent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	v, err := store.EventDetail(id)
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "detail": v})
}

func handleUpdateEvent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	var req EventPatch
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if err := store.UpdateEvent(id, req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	store.RecordAudit("update", "event", id, memberID(r), "更新事件资料")
	writeJSON(w, map[string]any{"ok": true})
}

// handleEventReport 与 handleGetEvent 返回内容相同，直接复用，避免两份逻辑各自漂移。
func handleEventReport(w http.ResponseWriter, r *http.Request) {
	handleGetEvent(w, r)
}

func handleActivateBaby(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	if err := store.ActivateBaby(id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	store.RecordAudit("activate", "baby_profile", id, memberID(r), "切换当前宝宝")
	writeJSON(w, map[string]any{"ok": true})
}

func handleListBabies(w http.ResponseWriter, r *http.Request) {
	list, err := store.ListBabies()
	if err != nil {
		writeErr(w, 500, "读取宝宝档案失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "babies": orEmpty(list)})
}

func handleCreateBabyProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Birthday string `json:"birthday"`
		Note     string `json:"note"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	id, err := store.AddBabyProfile(req.Name, req.Birthday, req.Note)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	store.RecordAudit("create", "baby_profile", id, memberID(r), req.Name)
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func handleUpdateBabyProfile(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	var req struct {
		Name     string `json:"name"`
		Birthday string `json:"birthday"`
		Note     string `json:"note"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if err := store.UpdateBabyProfile(id, req.Name, req.Birthday, req.Note); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	store.RecordAudit("update", "baby_profile", id, memberID(r), req.Name)
	writeJSON(w, map[string]any{"ok": true})
}

func handleRelationships(w http.ResponseWriter, r *http.Request) {
	list, err := store.RelationshipInsights(atoiDefault(r.URL.Query().Get("limit"), 200))
	if err != nil {
		writeErr(w, 500, "关系分析失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "relationships": orEmpty(list)})
}

func handleListTasks(w http.ResponseWriter, r *http.Request) {
	list, err := store.ListTasks(r.URL.Query().Get("status"), atoiDefault(r.URL.Query().Get("limit"), 200))
	if err != nil {
		writeErr(w, 500, "读取家庭事项失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "tasks": orEmpty(list)})
}

func handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title, Note, Kind, Due string
		AssigneeID             int64 `json:"assignee_id"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	id, err := store.AddTask(req.Title, req.Note, req.Kind, req.Due, req.AssigneeID, memberID(r))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	store.RecordAudit("create", "task", id, memberID(r), req.Title)
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	var req struct {
		Title, Note, Kind, Due, Status string
		AssigneeID                     int64 `json:"assignee_id"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if err := store.UpdateTask(id, req.Title, req.Note, req.Kind, req.Due, req.AssigneeID, req.Status); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	store.RecordAudit("update", "task", id, memberID(r), req.Status)
	writeJSON(w, map[string]any{"ok": true})
}

func handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	if err := store.DeleteTask(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	store.RecordAudit("delete", "task", id, memberID(r), "删除家庭事项")
	writeJSON(w, map[string]any{"ok": true})
}

func handleListShopping(w http.ResponseWriter, r *http.Request) {
	list, err := store.ListShopping(atoiDefault(r.URL.Query().Get("limit"), 200))
	if err != nil {
		writeErr(w, 500, "读取采购清单失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "items": orEmpty(list)})
}

func handleCreateShopping(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name, Category, Unit, Note string
		Qty                        float64
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	id, err := store.AddShopping(req.Name, req.Category, req.Qty, req.Unit, req.Note, memberID(r))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	store.RecordAudit("create", "shopping", id, memberID(r), req.Name)
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func handleUpdateShopping(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	var req struct {
		Name, Category, Unit, Note string
		Qty                        float64
		Checked                    bool
		BoughtByID                 int64 `json:"bought_by_id"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Checked {
		req.BoughtByID = memberID(r)
	} else {
		req.BoughtByID = 0
	}
	if err := store.UpdateShopping(id, req.Name, req.Category, req.Qty, req.Unit, req.Note, req.Checked, req.BoughtByID); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	store.RecordAudit("update", "shopping", id, memberID(r), req.Name)
	writeJSON(w, map[string]any{"ok": true})
}

func handleDeleteShopping(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	if err := store.DeleteShopping(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	store.RecordAudit("delete", "shopping", id, memberID(r), "删除采购项")
	writeJSON(w, map[string]any{"ok": true})
}

func handleListAudit(w http.ResponseWriter, r *http.Request) {
	list, err := store.ListAudit(atoiDefault(r.URL.Query().Get("limit"), 200))
	if err != nil {
		writeErr(w, 500, "读取操作记录失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "logs": orEmpty(list)})
}

func handleListAttachments(w http.ResponseWriter, r *http.Request) {
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	id, err := pathIDFromQuery(r.URL.Query().Get("record_id"))
	if kind == "" || err != nil {
		writeErr(w, 400, "附件关联信息不完整")
		return
	}
	list, err := store.ListAttachments(kind, id)
	if err != nil {
		writeErr(w, 500, "读取附件失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "attachments": orEmpty(list)})
}

func handleCreateAttachment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind     string `json:"kind"`
		FileName string `json:"file_name"`
		Mime     string `json:"mime"`
		Data     string `json:"data"`
		RecordID int64  `json:"record_id"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if len(req.Data) > 14*1024*1024 {
		writeErr(w, 413, "附件不能超过 10MB")
		return
	}
	dataText := req.Data
	if i := strings.Index(dataText, ","); i >= 0 {
		dataText = dataText[i+1:]
	}
	data, err := base64.StdEncoding.DecodeString(dataText)
	if err != nil {
		writeErr(w, 400, "附件内容不合法")
		return
	}
	if len(data) > 10*1024*1024 {
		writeErr(w, 413, "附件不能超过 10MB")
		return
	}
	id, err := store.AddAttachment(req.Kind, req.RecordID, strings.TrimSpace(req.FileName), strings.TrimSpace(req.Mime), data, memberID(r))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	store.RecordAudit("create", "attachment", id, memberID(r), req.FileName)
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func handleGetAttachment(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	v, data, err := store.AttachmentData(id)
	if err != nil {
		writeErr(w, 404, "附件不存在")
		return
	}
	w.Header().Set("Content-Type", v.Mime)
	w.Header().Set("Content-Disposition", attachmentDisposition(v.FileName))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	_, _ = w.Write(data)
}

func handleDeleteAttachment(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	if err := store.DeleteAttachment(id); err != nil {
		writeErr(w, 500, "删除附件失败")
		return
	}
	store.RecordAudit("delete", "attachment", id, memberID(r), "删除附件")
	writeJSON(w, map[string]any{"ok": true})
}

func handleAnnualReport(w http.ResponseWriter, r *http.Request) {
	v, err := store.AnnualReportFor(r.URL.Query().Get("year"), parseIDDefault(r.URL.Query().Get("baby_id"), 0))
	if err != nil {
		writeErr(w, 500, "生成年度报告失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "report": v})
}

func handleImportCSV(w http.ResponseWriter, r *http.Request) {
	var req struct{ Book, CSV string }
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Book != "gift" && req.Book != "baby" {
		writeErr(w, 400, "账本类型不合法")
		return
	}
	reader := csv.NewReader(strings.NewReader(req.CSV))
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, 400, "CSV 格式不合法")
		return
	}
	n, err := store.ImportCSVRows(req.Book, rows, memberID(r))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	store.RecordAudit("import", "csv", 0, memberID(r), req.Book)
	writeJSON(w, map[string]any{"ok": true, "count": n})
}

// attachmentDisposition 生成附件下载头：去掉引号/换行/反斜杠，
// 并用 RFC 5987 的 filename* 带上 UTF-8 名字，否则中文文件名会显示成一串乱码。
func attachmentDisposition(fileName string) string {
	name := strings.Map(func(r rune) rune {
		switch r {
		case '"', '\r', '\n', '\\':
			return -1
		}
		return r
	}, strings.TrimSpace(fileName))
	if name == "" {
		return "inline"
	}
	var ascii strings.Builder
	for _, r := range name {
		if r >= 32 && r < 127 {
			ascii.WriteRune(r)
		} else {
			ascii.WriteRune('_')
		}
	}
	return `inline; filename="` + ascii.String() + `"; filename*=UTF-8''` + url.QueryEscape(name)
}

func pathIDFromQuery(v string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("bad id")
	}
	return id, nil
}
