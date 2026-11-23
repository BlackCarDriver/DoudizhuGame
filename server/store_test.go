package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreDailyGiftOnlyOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "players.json")
	s := NewStore(path, 100, 10)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if got := s.Touch("alice", "a.png"); got != 110 {
		t.Fatalf("首次进入应 100+10=110，得到 %d", got)
	}
	if got := s.Touch("alice", "a.png"); got != 110 {
		t.Fatalf("同一天第二次进入不应重复赠送，得到 %d", got)
	}
	if got := s.Points("alice"); got != 110 {
		t.Fatalf("Points 应为 110，得到 %d", got)
	}
}

func TestStoreApplyResultAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "players.json")
	s := NewStore(path, 100, 0)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	s.Touch("alice", "a.png")
	s.Touch("bob", "b.png")
	s.ApplyResult(
		map[string]int{"alice": 6, "bob": -6},
		map[string]bool{"alice": true, "bob": false},
		map[string]string{"alice": "a.png", "bob": "b.png"},
	)
	if got := s.Points("alice"); got != 106 {
		t.Fatalf("alice 应 106，得到 %d", got)
	}
	if got := s.Points("bob"); got != 94 {
		t.Fatalf("bob 应 94，得到 %d", got)
	}

	s2 := NewStore(path, 100, 0)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	if got := s2.Points("alice"); got != 106 {
		t.Fatalf("重载后 alice 应 106，得到 %d", got)
	}
	if got := s2.Points("bob"); got != 94 {
		t.Fatalf("重载后 bob 应 94，得到 %d", got)
	}
}

func TestStoreSnapshotOrder(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "players.json"), 0, 0)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	s.ApplyResult(map[string]int{"alice": 10, "bob": 30, "carol": 20}, map[string]bool{}, nil)
	snap := s.Snapshot(10, "carol")
	rows, _ := snap["rows"].([]RankRow)
	if len(rows) != 3 {
		t.Fatalf("应有 3 行，得到 %d", len(rows))
	}
	if rows[0].Nick != "bob" || rows[1].Nick != "carol" || rows[2].Nick != "alice" {
		t.Fatalf("排序错误：%+v", rows)
	}
	me, _ := snap["me"].(*RankRow)
	if me == nil || me.Nick != "carol" || me.Rank != 2 {
		t.Fatalf("我的排名错误：%+v", me)
	}
}

func TestStoreCorruptFileStartsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "players.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewStore(path, 100, 0)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if got := s.Points("nobody"); got != 100 {
		t.Fatalf("坏档后应从空档开始，初始积分 100，得到 %d", got)
	}
}

func TestStoreRecoversFromBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "players.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	bak := `{"version":1,"players":{"alice":{"nick":"alice","points":77}}}`
	if err := os.WriteFile(path+".bak", []byte(bak), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewStore(path, 100, 0)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if got := s.Points("alice"); got != 77 {
		t.Fatalf("应从 .bak 恢复 alice=77，得到 %d", got)
	}
}
