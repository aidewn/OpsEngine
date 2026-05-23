package main

import (
	"OpsEngine/internal/api"
	"OpsEngine/internal/store"
	"fmt"
	"os"
	"sync"

	"go.uber.org/zap"
)

func main() {

	// 初始化日志
	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	zap.L().Info("OpsEngine 启动中...")

	// 初始化各组件
	workflowStore := store.NewWorkflowStore("data/workflows")

	checkDataDir([]string{"data/workflows", "data/executions", "data/logs"})

	var appState = &api.AppState{
		WorkflowStore: workflowStore,
		RunningMu:     sync.Mutex{},
		EngineAddr:    "",
	}

	var router = api.NewRouter(appState)
	var addr_port string = ":10001"
	zap.L().Info("服务已启动", zap.String("addr", fmt.Sprintf("http://localhost%s", addr_port)))
	router.Run(addr_port)
}

// 确保数据目录存在
func checkDataDir(dataDir []string) {
	for _, dir := range dataDir {
		if err := os.MkdirAll(dir, 0755); err != nil {
			zap.L().Fatal("创建目录失败", zap.Error(err))
		}
	}
}
