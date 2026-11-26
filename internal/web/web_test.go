package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gotodo/internal/task"
)

var now = time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)

func writeBoard(t *testing.T) (string, map[string]string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "board.json")
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	b.SetColumns(task.DevColumns)
	b.SetWIPLimit(task.StatusInDev, 1)
	ids := map[string]string{}
	xss := b.Add(`<script>alert("x")</script>`, "", nil, now).ID
	soon := now.Add(48 * time.Hour)
	late := now.Add(-72 * time.Hour)
	far := now.Add(10 * 24 * time.Hour)
	ids["xss"] = xss
	ids["soon"] = b.Add("Soon task", "", &soon, now).ID
	ids["late"] = b.Add("Late task", "", &late, now).ID
	ids["far"] = b.Add("Far task", "", &far, now).ID
	_ = b.Move(ids["late"], task.StatusInDev, now)
	_ = b.SetPR(ids["late"], "https://github.com/o/r/pull/9", now)
	_ = b.SetPR(ids["soon"], "#5", now)
	for i := range b.Tasks {
		switch b.Tasks[i].ID {
		case ids["late"]:
			until := now.Add(time.Hour)
			b.Tasks[i].ClaimedBy, b.Tasks[i].ClaimUntil = "claude", &until
			b.Tasks[i].Priority = task.PriorityHigh
			b.Tasks[i].Labels = []string{"<b>api</b>", "backend"}
		}
	}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	return path, ids
}

func get(t *testing.T, h http.Handler, method, target, host string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPageEscapesAndRendersCards(t *testing.T) {
	path, _ := writeBoard(t)
	h := (&Server{Path: path, Now: func() time.Time { return now }}).Handler()
	rec := get(t, h, http.MethodGet, "/", "127.0.0.1:7777")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if strings.Contains(body, `<script>alert`) || strings.Contains(body, "<b>api</b>") {
		t.Fatal("task text not escaped")
	}
	for _, want := range []string{
		"&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;",
		"&lt;b&gt;api&lt;/b&gt;",
		`class="cd u-overdue">late 3d<`,
		`class="cd u-soon">2d<`,
		`class="cd u-future">10d<`,
		"claimed by claude until 26/11/2026 10:00",
		`href="https://github.com/o/r/pull/9"`,
		"PR #5",
		`class="p-high">high<`,
		"due 23/11/2026",
		"IN DEV", "SHIPPED",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'nonce-") || !strings.Contains(body, `<script nonce="`) {
		t.Errorf("CSP nonce missing: %q", csp)
	}
}

func TestAPIBoardJSONShape(t *testing.T) {
	path, ids := writeBoard(t)
	h := (&Server{Path: path, Now: func() time.Time { return now }}).Handler()
	rec := get(t, h, http.MethodGet, "/api/board", "localhost:7777")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("status %d type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	var v View
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Kind != "development" || len(v.Columns) != 5 || v.Version == "" || v.GeneratedAt != "26/11/2026 09:00" {
		t.Fatalf("view = %+v", v)
	}
	indev := v.Columns[1]
	if indev.Status != "indev" || indev.Role != "active" || indev.WIPLimit != 1 || len(indev.Cards) != 1 {
		t.Fatalf("indev column = %+v", indev)
	}
	c := indev.Cards[0]
	if c.ID != ids["late"] || c.Countdown != "late 3d" || c.Urgency != "overdue" || c.PRURL == "" ||
		c.ClaimedBy != "claude" || c.Priority != "high" || c.Deadline != "23/11/2026" {
		t.Fatalf("card = %+v", c)
	}
	if v.Columns[4].Cards == nil {
		t.Fatal("empty column should encode cards as [], not null")
	}
	// Raw shape: keys agents rely on.
	var raw map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &raw)
	for _, k := range []string{"board", "columns", "version", "generated_at", "require_approval"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("JSON missing %q", k)
		}
	}
}

func TestVersionChangesWithBoardAndReloadsFromDisk(t *testing.T) {
	path, ids := writeBoard(t)
	s := &Server{Path: path, Now: func() time.Time { return now }}
	first, err := s.load()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := task.Load(path)
	_ = b.Move(ids["far"], task.StatusBlocked, now)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	second, _ := s.load()
	if first.Version == second.Version {
		t.Fatal("version did not change after a move on disk")
	}
	third, _ := s.load()
	if third.Version != second.Version {
		t.Fatal("version unstable without changes")
	}
}

func TestReadOnlyAndHostGuard(t *testing.T) {
	path, _ := writeBoard(t)
	h := (&Server{Path: path}).Handler()
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		if rec := get(t, h, m, "/api/board", "127.0.0.1:7777"); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s = %d, want 405", m, rec.Code)
		}
	}
	if rec := get(t, h, http.MethodGet, "/", "evil.example:7777"); rec.Code != http.StatusForbidden {
		t.Errorf("rebinding host = %d, want 403", rec.Code)
	}
	if rec := get(t, h, http.MethodGet, "/", "[::1]:7777"); rec.Code != http.StatusOK {
		t.Errorf("::1 host = %d, want 200", rec.Code)
	}
	if rec := get(t, h, http.MethodGet, "/nope", "127.0.0.1"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown path = %d, want 404", rec.Code)
	}
}

func TestMissingBoardRendersEmpty(t *testing.T) {
	h := (&Server{Path: filepath.Join(t.TempDir(), "none.json")}).Handler()
	if rec := get(t, h, http.MethodGet, "/", "127.0.0.1"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "TODO") {
		t.Fatalf("missing board = %d", rec.Code)
	}
}

func TestValidateAddr(t *testing.T) {
	for _, ok := range []string{"127.0.0.1:7777", "127.0.0.2:80", "localhost:7777", "[::1]:7777", "127.0.0.1:0"} {
		if err := ValidateAddr(ok); err != nil {
			t.Errorf("ValidateAddr(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{":7777", "0.0.0.0:7777", "[::]:7777", "192.168.1.5:7777", "example.com:80", "127.0.0.1", "127.0.0.1:http-x", "127.0.0.1:99999"} {
		if err := ValidateAddr(bad); err == nil {
			t.Errorf("ValidateAddr(%q) accepted", bad)
		}
	}
}

func TestServeEphemeralPortAndShutdown(t *testing.T) {
	path, _ := writeBoard(t)
	ctx, cancel := context.WithCancel(context.Background())
	urls := make(chan string, 1)
	errc := make(chan error, 1)
	go func() {
		errc <- Serve(ctx, "127.0.0.1:0", (&Server{Path: path}).Handler(), func(u string) { urls <- u })
	}()
	var url string
	select {
	case url = <-urls:
	case err := <-errc:
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Fatalf("url = %q", url)
	}
	resp, err := http.Get(url + "api/board")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"columns"`) {
		t.Fatalf("GET = %d %s", resp.StatusCode, body)
	}
	cancel()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("shutdown = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down")
	}
	if err := Serve(context.Background(), "0.0.0.0:0", http.NotFoundHandler(), nil); err == nil {
		t.Fatal("Serve bound a non-loopback address")
	}
}
