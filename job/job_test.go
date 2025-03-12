package job

import (
	"errors"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/dbtools"
	"github.com/ygpkg/yg-go/logs"
)

func TestRegistryCronFunc(t *testing.T) {

	dbtools.InitMutilMySQL(map[string]string{
		"core": "",
	})
	InitDB(dbtools.Core())
	// 注册定时任务
	RegistryCronFunc(dbtools.Core(), "*/2 * * * * *", 0, 0, "SyncLLMModelAndAccount", func() (string, error) {
		SyncLLMModelAndAccountToLLMModelHealth()
		return "Synchronization complete", nil
	})

	// 等待足够的时间让定时任务执行
	logs.Info("Wait 5 seconds to watch the scheduled task execute...")
	time.Sleep(5 * time.Second)

	logs.Info("End of test")
}

func SyncLLMModelAndAccountToLLMModelHealth() {
	logs.Info("Execute the LLMModel and Account synchronization tasks....")
}

// **测试定时任务失败**
func TestRegistryCronFunc_Failure(t *testing.T) {
	// **初始化数据库**（此处模拟数据库初始化失败）
	dbtools.InitMutilMySQL(map[string]string{
		"core": "",
	})
	InitDB(dbtools.Core())

	// **注册定时任务（任务返回错误）**
	RegistryCronFunc(dbtools.Core(), "*/2 * * * * *", 0, 0, "SyncLLMModelAndAccount_Failure", func() (string, error) {
		logs.Errorf("Execution failed test: LLMModel and Account synchronization tasks...")
		return "", errors.New("Simulation task failure: data synchronization error")
	})

	// **等待足够的时间让定时任务执行**
	logs.Info("Wait 5 seconds to see if the scheduled task fails...")
	time.Sleep(5 * time.Second)

	logs.Info("End of test")
}

// **测试定时任务崩溃**
func TestRegistryCronFunc_Panic(t *testing.T) {
	// **初始化数据库**（此处模拟数据库初始化失败）
	dbtools.InitMutilMySQL(map[string]string{
		"core": "",
	})
	InitDB(dbtools.Core())
	// **注册定时任务（任务发生 panic）**
	RegistryCronFunc(dbtools.Core(), "*/2 * * * * *", 0, 0, "SyncLLMModelAndAccount_Panic", func() (string, error) {
		logs.Errorf("Run crash tests: LLMModel and Account sync tasks...")
		panic("Simulated task crash: code exception")
	})

	// **等待足够的时间让定时任务执行**
	logs.Info("Wait 5 seconds to see if the task crashes...")
	time.Sleep(5 * time.Second)

	logs.Info("End of test")
}
