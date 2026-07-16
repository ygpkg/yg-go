package v2

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
)

type DriverFactory func(dsn string) (gorm.Dialector, error)

var drivers = map[string]DriverFactory{}

func Register(scheme string, factory DriverFactory) {
	drivers[strings.ToLower(scheme)] = factory
}

func Open(dburl string) (*gorm.DB, error) {
	dsn, err := url.Parse(dburl)
	if err != nil {
		logs.Errorf("[dbv2] parse dburl(%s) failed: %s", dburl, err)
		return nil, err
	}
	scheme := strings.ToLower(dsn.Scheme)
	factory, ok := drivers[scheme]
	if !ok {
		logs.Errorf("[dbv2] unsupported db scheme %q, registered: %v", scheme, driverNames())
		return nil, fmt.Errorf("unsupported db scheme %q", scheme)
	}
	dialector, err := factory(dburl)
	if err != nil {
		logs.Errorf("[dbv2] create dialector for %q failed: %s", scheme, err)
		return nil, err
	}
	db, err := gorm.Open(dialector, &gorm.Config{
		CreateBatchSize: 200,
	})
	if err != nil {
		logs.Errorf("[dbv2] open %q failed: %s", scheme, err)
		return nil, err
	}
	logs.Infof("[dbv2] open %q success", scheme)
	return db, nil
}

func driverNames() []string {
	names := make([]string, 0, len(drivers))
	for k := range drivers {
		names = append(names, k)
	}
	return names
}
