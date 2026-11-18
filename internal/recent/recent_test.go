package recent

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRecent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "recent")
	if got := Load(p); len(got) != 0 {
		t.Fatalf("missing file: %v", got)
	}
	Touch(p, "page/a")
	Touch(p, "page/b")
	Touch(p, "page/a")
	if got := Load(p); !reflect.DeepEqual(got, []string{"page/a", "page/b"}) {
		t.Fatalf("touch: %v", got)
	}
	Rename(p, "page/b", "page/c")
	Forget(p, "page/a")
	if got := Load(p); !reflect.DeepEqual(got, []string{"page/c"}) {
		t.Fatalf("rename/forget: %v", got)
	}
	for i := 0; i < Limit+5; i++ {
		Touch(p, fmt.Sprintf("page/n%d", i))
	}
	if got := Load(p); len(got) != Limit || got[0] != fmt.Sprintf("page/n%d", Limit+4) {
		t.Fatalf("cap: %d %v", len(got), got[0])
	}
	if err := Touch("", "page/x"); err != nil {
		t.Fatal("empty path should be a no-op")
	}
}
