package mysql

import (
	"os"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestCache(t *testing.T) {
	dburl := os.Getenv("TEST_DBURL")
	if dburl == "" {
		return
	}

	db, err := gorm.Open(mysql.Open(dburl), &gorm.Config{
		CreateBatchSize: 200,
	})
	if err != nil {
		t.Fatal(err)
	}
	db = db.Debug()

	c := NewCache(db)
	err = c.Set("key string", "val interface{}", time.Second*3)
	if err != nil {
		t.Fatal(err)
	}
}
