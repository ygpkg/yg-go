package mysqldrv

import (
	"net/url"

	dbtools "github.com/ygpkg/yg-go/dbtools/v2"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func init() {
	dbtools.Register("mysql", func(dsn string) (gorm.Dialector, error) {
		parsed, err := url.Parse(dsn)
		if err != nil {
			return nil, err
		}
		uri, err := normalizeMySQL(parsed)
		if err != nil {
			return nil, err
		}
		return mysql.Open(uri), nil
	})
}

func normalizeMySQL(u *url.URL) (string, error) {
	user := u.User.Username()
	pass, _ := u.User.Password()
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "3306"
	}
	db := ""
	if len(u.Path) > 1 {
		db = u.Path[1:]
	}
	query := u.RawQuery
	if query != "" {
		query = "?" + query
	}
	return user + ":" + pass + "@tcp(" + host + ":" + port + ")/" + db + query, nil
}
