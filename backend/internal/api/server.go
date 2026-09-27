// Package api is the HTTP surface of contract/api.md: the public SDK API, the admin API and the admin SPA.
package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"wishbone/backend/internal/compile"
	"wishbone/backend/internal/ops"
	"wishbone/backend/internal/preview"
	"wishbone/backend/internal/store"
)

type Config struct {
	Contract    *compile.Contract
	ContractDir string
	AdminDir    string // built admin SPA (admin/dist)
	Store       *store.Store
	Ops         map[string]ops.Handler
	Hub         *preview.Hub
	Password    string
}

type Server struct {
	Config
	errs      map[string]errInfo
	themes    map[string]theme
	templates map[string][]byte // template sources for GET /admin/api/templates/{name}
	catalog   []byte
}

type errInfo struct {
	HTTP      int
	Retryable bool
}

type theme struct {
	body    []byte
	version string
}

// Admin-only codes that contract/errors.json does not define.
var extraErrs = map[string]errInfo{"NOT_FOUND": {404, false}, "CONFLICT": {409, false}}

func New(cfg Config) (*Server, error) {
	s := &Server{Config: cfg, themes: map[string]theme{}, templates: map[string][]byte{}}
	var errFile struct {
		Codes map[string]struct {
			HTTP      int
			Retryable bool
		}
	}
	if err := readJSON(filepath.Join(cfg.ContractDir, "errors.json"), &errFile); err != nil {
		return nil, err
	}
	s.errs = maps.Clone(extraErrs)
	for code, e := range errFile.Codes {
		s.errs[code] = errInfo(e)
	}
	files, _ := filepath.Glob(filepath.Join(cfg.ContractDir, "tokens", "*.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		sum := sha256.Sum256(b)
		t := theme{version: hex.EncodeToString(sum[:8])}
		m["version"], _ = json.Marshal(t.version)
		t.body, _ = json.Marshal(m)
		s.themes[strings.TrimSuffix(filepath.Base(f), ".json")] = t
	}
	if _, ok := s.themes["korpu"]; !ok {
		return nil, errors.New("contract: tokens/korpu.json (the fallback theme) is missing")
	}
	for name := range cfg.Contract.Templates {
		b, err := os.ReadFile(filepath.Join(cfg.ContractDir, "templates", name+".json"))
		if err != nil {
			return nil, err
		}
		s.templates[name] = b
	}
	var err error
	if s.catalog, err = s.buildCatalog(); err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	return s, nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(b, v)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/screens/{id}", s.getScreen)
	mux.HandleFunc("POST /v1/data", s.postData)
	mux.HandleFunc("POST /v1/call", s.postCall)
	mux.HandleFunc("GET /v1/theme", s.getTheme)
	mux.HandleFunc("GET /v1/preview/sessions", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.Hub.Sessions()) })
	mux.HandleFunc("GET /v1/preview/ws", func(w http.ResponseWriter, r *http.Request) {
		if !s.Hub.ServeDevice(w, r, r.URL.Query().Get("session"), base(r)) {
			s.fail(w, "NOT_FOUND", "preview session not found")
		}
	})
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(filepath.Join(s.ContractDir, "assets")))))

	admin := func(pattern string, h http.HandlerFunc) { mux.HandleFunc(pattern, s.auth(h)) }
	admin("GET /admin/api/screens", s.listScreens)
	admin("POST /admin/api/screens", s.createScreen)
	admin("GET /admin/api/screens/{id}", s.getDraft)
	admin("PUT /admin/api/screens/{id}", s.putDraft)
	admin("POST /admin/api/screens/{id}/publish", s.publish)
	admin("GET /admin/api/screens/{id}/revisions", s.revisions)
	admin("POST /admin/api/screens/{id}/rollback/{rev}", s.rollback)
	admin("POST /admin/api/compile", s.compileSource)
	admin("GET /admin/api/catalog", func(w http.ResponseWriter, r *http.Request) { writeRaw(w, 200, s.catalog) })
	admin("GET /admin/api/templates/{name}", s.template)
	admin("POST /admin/api/preview", s.createPreview)
	admin("POST /admin/api/preview/{code}/document", s.previewDocument)
	admin("POST /admin/api/preview/{code}/play", s.previewPlay)
	admin("GET /admin/api/preview/{code}/ws", func(w http.ResponseWriter, r *http.Request) {
		if !s.Hub.ServeAdmin(w, r, r.PathValue("code")) {
			s.fail(w, "NOT_FOUND", "preview session not found")
		}
	})
	mux.HandleFunc("GET /admin/api/", func(w http.ResponseWriter, r *http.Request) { s.fail(w, "NOT_FOUND", "no such admin API route") })
	mux.HandleFunc("GET /admin/", s.spa)
	mux.Handle("GET /admin", http.RedirectHandler("/admin/", http.StatusMovedPermanently))
	return logRequests(mux)
}

// auth checks the admin password (Bearer; WebSockets may pass ?token=) in constant time.
func (s *Server) auth(h http.HandlerFunc) http.HandlerFunc {
	want := sha256.Sum256([]byte(s.Password))
	return func(w http.ResponseWriter, r *http.Request) {
		tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok && strings.HasSuffix(r.URL.Path, "/ws") {
			tok = r.URL.Query().Get("token")
		}
		got := sha256.Sum256([]byte(tok))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			s.fail(w, "UNAUTHENTICATED", "Нужна авторизация")
			return
		}
		h(w, r)
	}
}

func (s *Server) spa(w http.ResponseWriter, r *http.Request) {
	fsys := os.DirFS(s.AdminDir)
	if _, err := fs.Stat(fsys, "index.html"); err != nil {
		http.Error(w, "admin UI is not built: run `npm run build` in admin/ (or pass -admin)", http.StatusNotFound)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/admin/")
	if st, err := fs.Stat(fsys, name); name == "" || err != nil || st.IsDir() {
		name = "index.html"
	}
	http.ServeFileFS(w, r, fsys, name)
}

var screenID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// --- responses ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		slog.Error("encode response", "err", err)
		status, b = 500, []byte(`{"error":{"code":"INTERNAL","message":"encode","retryable":true}}`)
	}
	writeRaw(w, status, b)
}

func writeRaw(w http.ResponseWriter, status int, b []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

// fail writes the error envelope with the status and retryable flag of code.
func (s *Server) fail(w http.ResponseWriter, code, msg string) {
	e, ok := s.errs[code]
	if !ok {
		e = s.errs["INTERNAL"]
	}
	writeJSON(w, e.HTTP, map[string]any{"error": ops.Error{Code: code, Message: msg, Retryable: e.Retryable}})
}

// internal logs err (never request bodies) and answers 500.
func (s *Server) internal(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	s.fail(w, "INTERNAL", "Что-то пошло не так, попробуйте ещё раз")
}

// decode reads a JSON body (≤ 4 MB) into v; on error it answers 400 ARGS_INVALID.
func (s *Server) decode(w http.ResponseWriter, r *http.Request, v any) bool {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(v)
	if err != nil && !errors.Is(err, io.EOF) { // an empty body leaves v as is
		s.fail(w, "ARGS_INVALID", "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

// base is scheme://host as the client reached us, so asset URLs work from simulators and emulators alike.
func base(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := r.Header.Get("X-Forwarded-Proto"); p == "http" || p == "https" {
		scheme = p
	}
	return scheme + "://" + r.Host
}

func user(r *http.Request) string {
	if a := r.Header.Get("Authorization"); a != "" {
		return a
	}
	return "anon"
}

// etagMatch reports whether an If-None-Match header lists etag (quoted, weak or bare).
func etagMatch(header, etag string) bool {
	for _, part := range strings.Split(header, ",") {
		if strings.Trim(strings.TrimPrefix(strings.TrimSpace(part), "W/"), `"`) == etag {
			return true
		}
	}
	return false
}

// sortedKeys never returns nil, so JSON gets [] rather than null.
func sortedKeys[V any](m map[string]V) []string {
	return append([]string{}, slices.Sorted(maps.Keys(m))...)
}

// --- request log: method, path (no query: it may carry the admin token), status, duration ---

type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func logRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &recorder{ResponseWriter: w, status: 200}
		h.ServeHTTP(rec, r)
		slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status, "ms", time.Since(start).Milliseconds())
	})
}
