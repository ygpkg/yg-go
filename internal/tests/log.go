package main

import (
	"bufio"
	"context"
	"io"
	"os"
	"time"

	"github.com/ygpkg/yg-go/lifecycle"
	"github.com/ygpkg/yg-go/logs"
)

func main() {
	go syncLogsRoutine(lifecycle.Std().Context(), "/tmp/test.log")

	lifecycle.Std().WaitExit()
}

// syncLogsRoutine 同步日志到CLS
func syncLogsRoutine(ctx context.Context, filename string) {
	f, err := os.OpenFile(filename, os.O_RDONLY, 0666)
	if err != nil {
		logs.Errorf("open log file failed, %s %v", filename, err)
		return
	}
	defer f.Close()

	// 定位到文件末尾
	_, err = f.Seek(0, io.SeekEnd)
	if err != nil {
		logs.Errorf("seek log file failed, %s %v", filename, err)
		return
	}

	logger := logs.Named(filename)
	reader := bufio.NewReader(f)
	// 逐行读取日志文件
	for {
		select {
		case <-ctx.Done():
			logs.Infof("sync logs routine exit, %s", filename)
			return
		default:
			line, err := reader.ReadString('\n')
			if err == io.EOF {
				// 暂时没有新内容，等待一段时间再尝试读取
				time.Sleep(1 * time.Second)
				continue
			} else if err != nil {
				logs.Errorf("read log file failed, %v", err)
				return
			}
			// 将日志写入到CLS
			logger.Infof(line)
		}
	}
}
