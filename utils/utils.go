package utils

import "time"

// Retry 重試
func Retry(attempts int, sleep int, fn func() (error, bool)) (err error) {
	var needContinue bool
	for i := 0; i < attempts; i++ {
		err, needContinue = fn()
		if !needContinue {
			return err
		}
		if sleep > 0 {
			time.Sleep(time.Duration(sleep) * time.Second)
		}
	}
	return
}
