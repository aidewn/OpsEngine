package api

import (
	"OpsEngine/internal/store"
	"sync"

	"github.com/gin-gonic/gin"
)

// AppState 所有路由共享的应用状态
type AppState struct {
	//Registry       *registry.Registry
	WorkflowStore *store.WorkflowStore
	//ExecutionStore *store.ExecutionStore
	//// 运行中的工作流，key: workflow_id
	//Running    map[string]*runtime.WorkflowExecutor
	RunningMu  sync.Mutex
	EngineAddr string
}

func NewRouter(state *AppState) *gin.Engine {
	r := gin.Default()
	r.SetTrustedProxies(nil) // 禁用代理信任

	// CORS（开发阶段允许所有来源）
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	api := r.Group("/api")

	// 工作流定义
	api.GET("/workflows", state.listWorkflows)
	api.GET("/workflows/:id", state.getWorkflow)
	api.POST("/workflows", state.createWorkflow)
	api.PUT("/workflows/:id", state.updateWorkflow)
	api.DELETE("/workflows/:id", state.deleteWorkflow)

	return r
}
