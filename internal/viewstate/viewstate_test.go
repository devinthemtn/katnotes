package viewstate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestViewState(t *testing.T) {
	p := filepath.Join(t.TempDir(), "view.json")
	if st := Load(p); len(st.Sort) != 0 || len(st.Searches) != 0 {
		t.Fatalf("empty: %+v", st)
	}
	SetSort(p, "page", "title")
	SaveSearch(p, Search{Name: "Work  week", Mode: "tags", Text: "work"})
	SaveSearch(p, Search{Name: "budget", Mode: "text", Text: "budget"})
	st, err := SaveSearch(p, Search{Name: "work week", Mode: "tags", Text: "work meeting"})
	if err != nil {
		t.Fatal(err)
	}
	if st.Sort["page"] != "title" || len(st.Searches) != 2 || st.Searches[0].Name != "budget" {
		t.Fatalf("state: %+v", st)
	}
	if s, ok := Load(p).FindSearch("WORK WEEK"); !ok || s.Text != "work meeting" {
		t.Fatalf("find: %+v %v", s, ok)
	}
	if _, err := SaveSearch(p, Search{Name: "x", Text: " "}); err == nil {
		t.Error("empty search saved")
	}
	if _, err := DeleteSearch(p, "budget"); err != nil {
		t.Fatal(err)
	}
	if _, err := DeleteSearch(p, "budget"); err == nil {
		t.Error("deleting twice succeeded")
	}
	os.WriteFile(p, []byte("{not json"), 0o644)
	if st := Load(p); st.Sort == nil || st.Searches == nil {
		t.Error("corrupt file should load as empty")
	}
}
