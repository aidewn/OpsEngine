package api

import (
	"OpsEngine/internal/core"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// 列出工作流
func (s *AppState) listWorkflows(c *gin.Context) {
	workflows, err := s.WorkflowStore.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, workflows)
}

// 获取工作流
func (s *AppState) getWorkflow(c *gin.Context) {
	wf, err := s.WorkflowStore.Get(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, wf)
}

// 创建工作流
func (s *AppState) createWorkflow(c *gin.Context) {
	var wf core.WorkflowDef
	if err := c.ShouldBindJSON(&wf); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 生成uuid
	wf.ID = uuid.New().String()

	// 初始化一个空的工作流，包含节点和连线配置
	wf.Nodes = []core.NodeInstance{}
	wf.Edges = []core.EdgeConfig{}

	if err := s.WorkflowStore.Save(wf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": wf.ID})
}

// 更新工作流
func (s *AppState) updateWorkflow(c *gin.Context) {
	var wf core.WorkflowDef
	if err := c.ShouldBindJSON(&wf); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	wf.ID = c.Param("id")
	if err := s.WorkflowStore.Save(wf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// 删除工作流
func (s *AppState) deleteWorkflow(c *gin.Context) {
	if err := s.WorkflowStore.Delete(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
