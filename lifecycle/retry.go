package lifecycle

import "time"

// Retry 重試
func Retry(interval time.Duration, retryTimes int, fn func() (needRetry bool, err error)) (err error) {
	var needContinue bool
	for i := 0; i < retryTimes; i++ {
		needContinue, err = fn()
		if !needContinue {
			return err
		}
		if interval > 0 {
			time.Sleep(interval)
		}
	}
	return
}
