package task

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizePR(t *testing.T) {
	ok := map[string]string{
		"":                                "",
		"42":                              "42",
		"#7":                              "7",
		" https://github.com/o/r/pull/3 ": "https://github.com/o/r/pull/3",
	}
	for in, want := range ok {
		got, err := NormalizePR(in)
		if err != nil || got != want {
			t.Errorf("NormalizePR(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"#", "abc", "javascript:alert(1)", "ftp://x/1", "12a", "https://"} {
		if _, err := NormalizePR(bad); err == nil {
			t.Errorf("NormalizePR(%q) = nil error, want rejection", bad)
		}
	}
}

func TestSetPRLinksClearsAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.json")
	b, _ := Load(path)
	id := b.Add("x", "", nil, ref).ID
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	later := ref.Add(time.Hour)
	if err := b.SetPR(id, "#12", later); err != nil {
		t.Fatal(err)
	}
	if !b.Dirty() {
		t.Fatal("SetPR did not mark the board dirty")
	}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	got, _ := Load(path)
	if tk, _ := got.TaskByID(id); tk.PR != "12" || !tk.UpdatedAt.Equal(later) {
		t.Fatalf("PR not persisted: %+v", tk)
	}
	if err := got.SetPR(id, "12", ref); err != nil || got.Dirty() {
		t.Fatalf("unchanged PR should be a no-op (err %v, dirty %v)", err, got.Dirty())
	}
	if err := got.SetPR(id, "", ref); err != nil {
		t.Fatal(err)
	}
	if tk, _ := got.TaskByID(id); tk.PR != "" {
		t.Fatalf("blank PR did not clear: %q", tk.PR)
	}
	if err := got.SetPR("nope", "1", ref); err != ErrNotFound {
		t.Fatalf("missing task = %v, want ErrNotFound", err)
	}
	if err := got.SetPR(id, "bogus", ref); err == nil {
		t.Fatal("bad PR accepted")
	}
}
