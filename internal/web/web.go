// Package web serves a read-only HTML view of a board on the loopback
// interface. It has no write endpoints and reloads the board from disk on
// every request.
package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"strings"
	"time"

	"gotodo/internal/task"
)

const dateLayout = "02/01/2006"

//go:embed board.html.tmpl
var templateFS embed.FS

var pageTemplate = template.Must(template.ParseFS(templateFS, "board.html.tmpl"))

// ValidateAddr accepts only loopback host:port addresses (127.0.0.0/8, ::1
// or localhost) so the board is never exposed on the network.
func ValidateAddr(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("bad -addr %q (want 127.0.0.1:PORT): %v", addr, err)
	}
	if _, err := net.LookupPort("tcp", port); err != nil || port == "" {
		return fmt.Errorf("bad -addr %q: invalid port", addr)
	}
	if !isLoopbackHost(host) {
		return fmt.Errorf("refusing -addr %q: serve binds loopback only (use 127.0.0.1:PORT)", addr)
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Server renders one board file.
type Server struct {
	Path string
	Now  func() time.Time // injectable clock; nil means time.Now
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Card is one task as the page and /api/board expose it.
type Card struct {
	ID         string   `json:"id"`
	ShortID    string   `json:"short_id"`
	Title      string   `json:"title"`
	Status     string   `json:"status"`
	Priority   string   `json:"priority,omitempty"`
	Labels     []string `json:"labels,omitempty"`
	Deadline   string   `json:"deadline,omitempty"` // DD/MM/YYYY
	Countdown  string   `json:"countdown,omitempty"`
	Urgency    string   `json:"urgency,omitempty"` // future, soon, urgent, overdue
	ClaimedBy  string   `json:"claimed_by,omitempty"`
	ClaimUntil string   `json:"claim_until,omitempty"` // DD/MM/YYYY HH:MM
	PR         string   `json:"pr,omitempty"`
	PRURL      string   `json:"pr_url,omitempty"` // only for http(s) PR links
	ParentID   string   `json:"parent_id,omitempty"`
	BlockedBy  []string `json:"blocked_by,omitempty"`
	LastAgent  string   `json:"last_agent,omitempty"`
}

// Column is one board column.
type Column struct {
	Status   string `json:"status"`
	Label    string `json:"label"`
	Role     string `json:"role"` // todo, active, blocked or done (for colour)
	WIPLimit int    `json:"wip_limit,omitempty"`
	Cards    []Card `json:"cards"`
}

// View is the whole page model and the /api/board payload.
type View struct {
	Kind            string `json:"board"` // personal or development
	RequireApproval bool   `json:"require_approval"`
	GeneratedAt     string `json:"generated_at"` // DD/MM/YYYY HH:MM
	// Version changes whenever anything shown changes; the page polls it
	// and reloads on a difference.
	Version string   `json:"version"`
	Columns []Column `json:"columns"`
}

func priorityName(p task.Priority) string {
	switch p {
	case task.PriorityHigh:
		return "high"
	case task.PriorityMedium:
		return "medium"
	case task.PriorityLow:
		return "low"
	}
	return ""
}

func urgencyName(u task.Urgency) string {
	switch u {
	case task.UrgencyFuture:
		return "future"
	case task.UrgencySoon:
		return "soon"
	case task.UrgencyUrgent:
		return "urgent"
	case task.UrgencyOverdue:
		return "overdue"
	}
	return ""
}

func role(b *task.Board, s task.Status) string {
	switch {
	case s == b.DoneStatus():
		return "done"
	case s == task.StatusBlocked:
		return "blocked"
	case b.ColumnIndex(s) == 0:
		return "todo"
	}
	return "active"
}

// countdown mirrors the TUI: "late 3d", "today", "2d".
func countdown(t task.Task, now time.Time, done task.Status) (string, string) {
	u := task.DeadlineUrgency(t, now, done)
	days, ok := task.DaysUntilDeadline(t, now)
	if !ok || u == task.UrgencyDone {
		return "", ""
	}
	switch {
	case days < 0:
		return fmt.Sprintf("late %dd", -days), urgencyName(u)
	case days == 0:
		return "today", urgencyName(u)
	}
	return fmt.Sprintf("%dd", days), urgencyName(u)
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// BuildView turns a board into the page model.
func BuildView(b *task.Board, now time.Time) View {
	done := b.DoneStatus()
	v := View{Kind: "personal", RequireApproval: b.RequireApproval, GeneratedAt: now.Format("02/01/2006 15:04")}
	if done == task.StatusShipped {
		v.Kind = "development"
	}
	for _, s := range b.Statuses() {
		col := Column{Status: string(s), Label: s.Label(), Role: role(b, s), WIPLimit: b.WIPLimits[s], Cards: []Card{}}
		for _, t := range b.ByStatus(s) {
			c := Card{
				ID: t.ID, ShortID: shortID(t.ID), Title: t.Title, Status: string(t.Status),
				Priority: priorityName(t.Priority), Labels: t.Labels, PR: t.PR,
				ParentID: t.ParentID, BlockedBy: t.BlockedBy, LastAgent: t.LastAgent,
			}
			if t.Deadline != nil {
				c.Deadline = t.Deadline.In(now.Location()).Format(dateLayout)
			}
			c.Countdown, c.Urgency = countdown(t, now, done)
			if t.ClaimedBy != "" && (t.ClaimUntil == nil || t.ClaimUntil.After(now)) {
				c.ClaimedBy = t.ClaimedBy
				if t.ClaimUntil != nil {
					c.ClaimUntil = t.ClaimUntil.In(now.Location()).Format("02/01/2006 15:04")
				}
			}
			if strings.HasPrefix(t.PR, "https://") || strings.HasPrefix(t.PR, "http://") {
				c.PRURL = t.PR
			}
			col.Cards = append(col.Cards, c)
		}
		v.Columns = append(v.Columns, col)
	}
	data, _ := json.Marshal(struct {
		Kind    string
		Gate    bool
		Columns []Column
	}{v.Kind, v.RequireApproval, v.Columns})
	sum := sha256.Sum256(data)
	v.Version = hex.EncodeToString(sum[:8])
	return v
}

// Handler routes "/" (HTML) and "/api/board" (JSON). Only GET and HEAD are
// allowed, and requests must name a loopback Host (DNS-rebinding guard).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.page)
	mux.HandleFunc("/api/board", s.api)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "read-only", http.StatusMethodNotAllowed)
			return
		}
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if !isLoopbackHost(strings.Trim(host, "[]")) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) load() (View, error) {
	b, err := task.Load(s.Path)
	if err != nil {
		return View{}, err
	}
	now := s.now()
	b.SweepArchive(now) // in memory only: serve never saves
	return BuildView(b, now), nil
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	v, err := s.load()
	if err != nil {
		http.Error(w, "cannot read board", http.StatusInternalServerError)
		return
	}
	nonce := newNonce()
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; style-src 'nonce-"+nonce+"'; script-src 'nonce-"+nonce+"'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pageTemplate.Execute(w, struct {
		View
		Nonce string
	}{v, nonce}); err != nil {
		// Headers are already sent; nothing more to do.
		return
	}
}

func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	v, err := s.load()
	if err != nil {
		http.Error(w, `{"error":"cannot read board"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func newNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// Serve listens on addr (validated loopback) and serves h until ctx is
// cancelled, then shuts down gracefully. ready receives the page URL once
// the listener is bound.
func Serve(ctx context.Context, addr string, h http.Handler, ready func(url string)) error {
	if err := ValidateAddr(addr); err != nil {
		return err
	}
	host, port, _ := net.SplitHostPort(addr)
	if strings.EqualFold(host, "localhost") {
		addr = net.JoinHostPort("127.0.0.1", port)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second}
	if ready != nil {
		ready("http://" + ln.Addr().String() + "/")
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
