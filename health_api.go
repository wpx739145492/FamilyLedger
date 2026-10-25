package main

import "net/http"

func handleListHealth(w http.ResponseWriter, r *http.Request) {
	list, err := store.ListHealth(parseIDDefault(r.URL.Query().Get("baby_id"), 1), atoiDefault(r.URL.Query().Get("limit"), 200))
	if err != nil {
		writeErr(w, 500, "读取健康记录失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "records": orEmpty(list)})
}

func handleCreateHealth(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BabyID int64  `json:"baby_id"`
		Kind   string `json:"kind"`
		Title  string `json:"title"`
		Date   string `json:"date"`
		Value  string `json:"value"`
		Note   string `json:"note"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	id, err := store.AddHealth(req.BabyID, req.Kind, req.Title, req.Date, req.Value, req.Note, memberID(r))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	store.RecordAudit("create", "health", id, memberID(r), req.Title)
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func handleDeleteHealth(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	if err := store.DeleteHealth(id); err != nil {
		writeErr(w, 500, "删除健康记录失败")
		return
	}
	store.RecordAudit("delete", "health", id, memberID(r), "删除健康记录")
	writeJSON(w, map[string]any{"ok": true})
}

func handleListReminders(w http.ResponseWriter, r *http.Request) {
	list, err := store.ListReminders(parseIDDefault(r.URL.Query().Get("baby_id"), 1), r.URL.Query().Get("status"), atoiDefault(r.URL.Query().Get("limit"), 200))
	if err != nil {
		writeErr(w, 500, "读取提醒失败")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "reminders": orEmpty(list)})
}

func handleCreateReminder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BabyID     int64  `json:"baby_id"`
		Kind       string `json:"kind"`
		Title      string `json:"title"`
		Due        string `json:"due"`
		Note       string `json:"note"`
		RepeatDays int    `json:"repeat_days"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	id, err := store.AddReminder(req.BabyID, req.Kind, req.Title, req.Due, req.RepeatDays, req.Note, memberID(r))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	store.RecordAudit("create", "reminder", id, memberID(r), req.Title)
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func handleUpdateReminder(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if err := store.UpdateReminder(id, req.Status); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	store.RecordAudit("update", "reminder", id, memberID(r), req.Status)
	writeJSON(w, map[string]any{"ok": true})
}

func handleDeleteReminder(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "id 不合法")
		return
	}
	if err := store.DeleteReminder(id); err != nil {
		writeErr(w, 500, "删除提醒失败")
		return
	}
	store.RecordAudit("delete", "reminder", id, memberID(r), "删除提醒")
	writeJSON(w, map[string]any{"ok": true})
}

func parseIDDefault(v string, def int64) int64 {
	id, err := pathIDFromQuery(v)
	if err != nil {
		return def
	}
	return id
}
