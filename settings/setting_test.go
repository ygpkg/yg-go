package settings

import (
	"context"
	"testing"

	dbtools "github.com/ygpkg/yg-go/dbtools/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB 使用内存 sqlite 注册 core 库，便于对 settings 做真实读写测试。
func setupTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %s", err)
	}
	if dbtools.DBExists("core") {
		// 已注册则直接复用，避免多次注册报错。
		return
	}
	dbtools.RegistryDB("core", db)
	if err := InitDB(); err != nil {
		t.Fatalf("init db failed: %s", err)
	}
}

func TestSetGetText(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	if err := SetText(ctx, "test", "foo", "bar"); err != nil {
		t.Fatalf("set text failed: %s", err)
	}
	val, err := GetText(ctx, "test", "foo")
	if err != nil {
		t.Fatalf("get text failed: %s", err)
	}
	if val != "bar" {
		t.Fatalf("expect bar, got %s", val)
	}
}

func TestSetGetYaml(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	type conf struct {
		A string `yaml:"a"`
		B int    `yaml:"b"`
	}
	if err := SetYaml(ctx, "test", "yaml-key", &conf{A: "x", B: 2}); err != nil {
		t.Fatalf("set yaml failed: %s", err)
	}
	var got conf
	if err := GetYaml(ctx, "test", "yaml-key", &got); err != nil {
		t.Fatalf("get yaml failed: %s", err)
	}
	if got.A != "x" || got.B != 2 {
		t.Fatalf("unexpected yaml value: %+v", got)
	}
}

func TestListAndUpdates(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	if err := SetText(ctx, "grp", "k1", "v1"); err != nil {
		t.Fatalf("set k1 failed: %s", err)
	}
	if err := SetText(ctx, "grp", "k2", "v2"); err != nil {
		t.Fatalf("set k2 failed: %s", err)
	}
	list, err := List(ctx, "grp", "k1", "k2")
	if err != nil {
		t.Fatalf("list failed: %s", err)
	}
	if len(list) != 2 {
		t.Fatalf("expect 2 items, got %d", len(list))
	}

	if err := Updates(ctx, &SettingItem{Group: "grp", Key: "k1", Value: "v1-new", ValueType: ValueText}); err != nil {
		t.Fatalf("updates failed: %s", err)
	}
	val, err := GetText(ctx, "grp", "k1")
	if err != nil {
		t.Fatalf("get after update failed: %s", err)
	}
	if val != "v1-new" {
		t.Fatalf("expect v1-new, got %s", val)
	}
}
