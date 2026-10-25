package main

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed web
var webFS embed.FS

var (
	store     *Store
	sharedTok string
	backupDir string
	failTrack = &failLimiter{m: map[string]*failRec{}}
)

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

type failRec struct {
	n     int
	until time.Time
}

type failLimiter struct {
	mu sync.Mutex
	m  map[string]*failRec
}

// allow 返回 false 表示当前被限流。失败 5 次后按指数退避。
func (f *failLimiter) allow(ip string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.m[ip]
	if !ok {
		return true
	}
	return time.Now().After(r.until)
}

func (f *failLimiter) fail(ip string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.m[ip]
	if !ok {
		r = &failRec{}
		f.m[ip] = r
	}
	r.n++
	// 第 1~4 次不罚，第 5 次起 2^(n-4) 秒，最长 10 分钟
	if r.n >= 5 {
		d := time.Duration(1<<min(r.n-4, 10)) * time.Second
		if d > 10*time.Minute {
			d = 10 * time.Minute
		}
		r.until = time.Now().Add(d)
	}
}

func (f *failLimiter) reset(ip string) {
	f.mu.Lock()
	delete(f.m, ip)
	f.mu.Unlock()
}

func main() {
	addr := envOr("FL_ADDR", ":8080")
	dbPath := envOr("FL_DB", "/data/familyledger.db")
	sharedTok = strings.TrimSpace(envOr("FL_SHARED_TOKEN", ""))
	backupDir = strings.TrimSpace(envOr("FL_BACKUP_DIR", ""))

	if sharedTok == "" {
		sharedTok = "123456"
		log.Println("[warn] FL_SHARED_TOKEN 未设置，已使用默认口令 123456，请尽快修改")
	}

	var err error
	if parent := filepath.Dir(dbPath); parent != "" {
		_ = os.MkdirAll(parent, 0o755)
	}
	store, err = Open(dbPath)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer store.Close()

	if backupDir != "" {
		_ = os.MkdirAll(backupDir, 0o755)
		go backupLoop()
	}

	mux := http.NewServeMux()
	registerRoutes(mux)

	srv := &http.Server{
		Addr:         addr,
		Handler:      logging(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	log.Printf("familyledger 启动中 db=%s addr=%s", dbPath, addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("服务退出: %v", err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", handleHealth)

	mux.HandleFunc("GET /api/state", handleState)
	mux.HandleFunc("POST /api/login", handleLogin)
	mux.HandleFunc("POST /api/session", handleSession)
	mux.HandleFunc("POST /api/logout", handleLogout)

	mux.HandleFunc("GET /api/members", auth(handleListMembers))
	mux.HandleFunc("POST /api/members", auth(handleAddMember))

	mux.HandleFunc("GET /api/persons", auth(handleSearchPersons))
	mux.HandleFunc("POST /api/persons", auth(handleCreatePerson))
	mux.HandleFunc("GET /api/persons/{id}", auth(handleGetPerson))
	mux.HandleFunc("PATCH /api/persons/{id}", auth(handleUpdatePerson))
	mux.HandleFunc("DELETE /api/persons/{id}", auth(handleDeletePerson))
	mux.HandleFunc("GET /api/persons/{id}/records", auth(handlePersonRecords))

	mux.HandleFunc("POST /api/gifts", auth(handleAddGift))
	mux.HandleFunc("POST /api/gifts/batch", auth(handleBatchGifts))
	mux.HandleFunc("PATCH /api/gifts/{id}", auth(handleUpdateGift))
	mux.HandleFunc("DELETE /api/gifts/{id}", auth(handleDeleteGift))
	mux.HandleFunc("GET /api/gifts", auth(handleListGifts))
	mux.HandleFunc("GET /api/stats", auth(handleStats))
	mux.HandleFunc("GET /api/backup.json", auth(handleBackup))
	mux.HandleFunc("POST /api/restore", auth(handleRestore))
	mux.HandleFunc("GET /api/gifts/summary", auth(handleGiftSummary))
	mux.HandleFunc("GET /api/owes", auth(handleOwes))

	mux.HandleFunc("GET /api/events", auth(handleListEvents))
	mux.HandleFunc("POST /api/events", auth(handleCreateEvent))
	mux.HandleFunc("GET /api/events/{id}", auth(handleGetEvent))
	mux.HandleFunc("PATCH /api/events/{id}", auth(handleUpdateEvent))
	mux.HandleFunc("GET /api/events/{id}/report", auth(handleEventReport))

	mux.HandleFunc("POST /api/baby", auth(handleAddBaby))
	mux.HandleFunc("PATCH /api/baby/{id}", auth(handleUpdateBaby))
	mux.HandleFunc("DELETE /api/baby/{id}", auth(handleDeleteBaby))
	mux.HandleFunc("GET /api/baby", auth(handleListBaby))
	mux.HandleFunc("GET /api/baby/summary", auth(handleBabySummary))

	mux.HandleFunc("GET /api/export.csv", auth(handleExportCSV))

	mux.HandleFunc("GET /api/insights", auth(handleInsights))
	mux.HandleFunc("GET /api/timeline", auth(handleTimeline))
	mux.HandleFunc("GET /api/consumables", auth(handleConsumables))

	mux.HandleFunc("GET /api/milestones", auth(handleListMilestones))
	mux.HandleFunc("POST /api/milestones", auth(handleAddMilestone))
	mux.HandleFunc("DELETE /api/milestones/{id}", auth(handleDeleteMilestone))
	mux.HandleFunc("GET /api/milestone-kinds", auth(handleMilestoneKinds))

	mux.HandleFunc("GET /api/profile", auth(handleGetProfile))
	mux.HandleFunc("POST /api/profile", auth(handleSetProfile))
	mux.HandleFunc("GET /api/babies", auth(handleListBabies))
	mux.HandleFunc("POST /api/babies", auth(handleCreateBabyProfile))
	mux.HandleFunc("PATCH /api/babies/{id}", auth(handleUpdateBabyProfile))
	mux.HandleFunc("POST /api/babies/{id}/activate", auth(handleActivateBaby))
	mux.HandleFunc("GET /api/health", auth(handleListHealth))
	mux.HandleFunc("POST /api/health", auth(handleCreateHealth))
	mux.HandleFunc("DELETE /api/health/{id}", auth(handleDeleteHealth))
	mux.HandleFunc("GET /api/reminders", auth(handleListReminders))
	mux.HandleFunc("POST /api/reminders", auth(handleCreateReminder))
	mux.HandleFunc("PATCH /api/reminders/{id}", auth(handleUpdateReminder))
	mux.HandleFunc("DELETE /api/reminders/{id}", auth(handleDeleteReminder))

	mux.HandleFunc("GET /api/relationships", auth(handleRelationships))
	mux.HandleFunc("GET /api/tasks", auth(handleListTasks))
	mux.HandleFunc("POST /api/tasks", auth(handleCreateTask))
	mux.HandleFunc("PATCH /api/tasks/{id}", auth(handleUpdateTask))
	mux.HandleFunc("DELETE /api/tasks/{id}", auth(handleDeleteTask))
	mux.HandleFunc("GET /api/shopping", auth(handleListShopping))
	mux.HandleFunc("POST /api/shopping", auth(handleCreateShopping))
	mux.HandleFunc("PATCH /api/shopping/{id}", auth(handleUpdateShopping))
	mux.HandleFunc("DELETE /api/shopping/{id}", auth(handleDeleteShopping))
	mux.HandleFunc("GET /api/audit", auth(handleListAudit))
	mux.HandleFunc("GET /api/attachments", auth(handleListAttachments))
	mux.HandleFunc("POST /api/attachments", auth(handleCreateAttachment))
	mux.HandleFunc("GET /api/attachments/{id}", auth(handleGetAttachment))
	mux.HandleFunc("DELETE /api/attachments/{id}", auth(handleDeleteAttachment))
	mux.HandleFunc("GET /api/report", auth(handleAnnualReport))
	mux.HandleFunc("POST /api/import/csv", auth(handleImportCSV))

	// 静态资源 + SPA 回退
	mux.Handle("/", spaHandler())
}

func spaHandler() http.Handler {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}
	fileSrv := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(sub, p); err == nil {
			fileSrv.ServeHTTP(w, r)
			return
		}
		// 未知路径一律返回首页，保证前端路由可用
		r.URL.Path = "/"
		fileSrv.ServeHTTP(w, r)
	})
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/healthz" {
			log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(t).Round(time.Millisecond))
		}
	})
}

// ---- 认证中间件 ----

type ctxKey int

const ctxMember ctxKey = 1

func currentMember(r *http.Request) *Member {
	m, _ := r.Context().Value(ctxMember).(*Member)
	return m
}

func auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tk := bearerToken(r)
		m, err := store.MemberByToken(tk)
		if err != nil || m == nil {
			writeErr(w, http.StatusUnauthorized, "登录已失效，请重新进入")
			return
		}
		ctx := contextWithMember(r.Context(), m)
		next(w, r.WithContext(ctx))
	}
}

func bearerToken(r *http.Request) string {
	if v := r.Header.Get("Authorization"); strings.HasPrefix(v, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(v, "Bearer "))
	}
	if c, err := r.Cookie("fl_token"); err == nil && c.Value != "" {
		return c.Value
	}
	// 供 CSV 下载这类直接跳转的场景兜底
	return r.URL.Query().Get("t")
}

// ---- API 基础读写 ----

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

type errResp struct {
	Error string `json:"error"`
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(errResp{Error: msg})
}

func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		return strings.Split(v, ",")[0]
	}
	return r.RemoteAddr
}

// ---- 备份 ----

func backupLoop() {
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
		if !next.After(now) {
			next = next.AddDate(0, 0, 1)
		}
		time.Sleep(next.Sub(now))
		runBackup()
	}
}

func runBackup() {
	if backupDir == "" {
		return
	}
	name := "fl-" + time.Now().Format("2006-01-02") + ".db"
	dst := filepath.Join(backupDir, name)
	if err := store.VacuumInto(dst); err != nil {
		log.Printf("[backup] 失败: %v", err)
		return
	}
	log.Printf("[backup] 已生成 %s", dst)
	pruneBackups(30)
}

func pruneBackups(keep int) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}
	var dbs []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "fl-") && strings.HasSuffix(e.Name(), ".db") {
			dbs = append(dbs, e)
		}
	}
	if len(dbs) <= keep {
		return
	}
	// 名称含日期，按字典序即时间序
	sortByName(dbs)
	for _, e := range dbs[:len(dbs)-keep] {
		_ = os.Remove(filepath.Join(backupDir, e.Name()))
	}
}

func sortByName(list []os.DirEntry) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && list[j].Name() < list[j-1].Name(); j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

func secureEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
