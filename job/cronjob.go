package job

import (
	"github.com/robfig/cron/v3"
)

var stdCron *cron.Cron

// RegistryCronFunc 注册一个定时任务
func RegistryCronFunc(spec string, f func()) {
	if stdCron == nil {
		stdCron = cron.New(cron.WithSeconds())
		stdCron.Start()
	}
	stdCron.AddFunc(spec, f)
}
