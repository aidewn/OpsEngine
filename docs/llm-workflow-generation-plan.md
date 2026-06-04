# 大模型生成工作流

> 本文档反映当前 master 上的实际实现，不再是规划稿。

## 总体能力

OpsEngine 集成了基于外部大模型的 AI 助手，统一入口是工作流列表页右上角的 **「AI 助手」** 按钮。
打开对话框后用户选择目标环境与 SSH 配置，再输入运维需求或对话，后端会自动判断：

```
用户输入
  -> 命中"工作流/巡检/流程/workflow/inspection/pipeline"等关键词 ──▶ 生成工作流
  -> 否则                                                          ──▶ 普通流式问答
```

所有结果通过 Wails 事件 `ai:assistant` 推送给前端，事件类型有 `delta / progress / workflow / done / error` 五种。

## 模型与配置

- **客户端**：[internal/clients/llm.go](../internal/clients/llm.go) `LLMClient`，OpenAI 兼容 `/chat/completions`，支持流式。
- **配置位置**：`data/settings/ai.toml`（已加入 `.gitignore`），通过「设置」tab 维护。
- **默认指向 DeepSeek**：常量 `DefaultDeepSeekBaseURL` / `DefaultDeepSeekModel`。把 `deepseek_base_url` 改成任何 OpenAI 兼容端点即可切换 provider（OpenAI、Qwen、本地 ollama 等）。

字段名为历史原因仍以 `deepseek_` 开头，但语义上是通用 LLM 设置。

## 工作流生成流程

实现位置：[ai.go](../ai.go) `startAIWorkflowGeneration`。

```
1. 收集上下文：GetNodeTypes() + ListEnvironments()，过滤敏感字段
2. 构造 system prompt：节点目录 JSON + 环境摘要 JSON + 用户偏好 JSON + 输出规范 + 硬约束
3. 调用 LLMClient.Chat（非流式）
4. parseGeneratedWorkflow：从回复中提取 { ... } 子串解析为 aiGeneratedWorkflow
5. materializeWorkflow：
   - 检查每个 type_id 在 engine.Lookup 或 assembleStore 中存在
   - 把临时 id 重写成 UUID，同步替换 edges 的引用
   - engine.ValidateWorkflow 做单例/exec 出入度/变量引用结构校验
6. workflowStore.Save 落盘
7. emit "workflow" 事件返回 workflow_id，让前端跳转到画布
```

模型必须输出的 JSON 结构：

```json
{
  "name": "工作流名称",
  "description": "一句话描述",
  "variables": [],
  "nodes": [
    {"id":"n1","type_id":"system_ready","config":{},"position":{"x":80,"y":120}}
  ],
  "edges": [
    {"from":{"node":"n1","port":"exec_out"},"to":{"node":"n2","port":"exec_in"}}
  ],
  "notes": []
}
```

## 注入 prompt 的上下文摘要

`summarizeNodeTypes` / `summarizeEnvironments` 把全量数据压缩成精简结构，避免泄露敏感字段：

- **节点目录**：`{type_id, name, kind, description, in:[{id,type}], out:[{id,type}], config:[{id,type,required,default}]}`
- **环境列表**：`{id, name, configs:[{id, name, kind}]}` —— 密码 / token / 私钥等不进 prompt
- **用户偏好**：`{environment_id, ssh_config_id}` —— 给模型选择 SSH 时一个明确目标

## 硬约束（写入 prompt）

```
- 必须有且仅有一个 system_ready 节点作为入口
- 只能使用注入的 type_id
- 临时 id 后端会替换为 UUID
- environment_id / config_id 必须在已配置环境中存在
- exec_out 输出端口最多 1 条出边；数据输入端口最多 1 条入边
- 端口 id 必须真实存在
- 不确定的字段留空写入 notes
- 不要包含 rm -rf / mkfs / shutdown 等破坏性命令
```

## 后端绑定 API

[app.go](../app.go) / [ai.go](../ai.go) 暴露给 Wails 的方法只有 4 个，前端通过 `@wails/go/main/App` 调用：

| 方法 | 用途 |
|------|------|
| `GetAISettings` | 读取本地配置 |
| `UpdateAISettings` | 保存配置 |
| `TestAISettings` | 用当前配置发一次"连接成功"测试 |
| `StartAIAssistant` | 统一对话入口，结果走 `ai:assistant` 事件 |

不再保留 `AIChat` / `GenerateWorkflowFromAI` 这类一次性 RPC（已并入 `StartAIAssistant`）。

## 测试

[ai_test.go](../ai_test.go) 覆盖纯函数：

- `parseGeneratedWorkflow`：从带噪声的回复中提取 JSON
- `materializeWorkflow`：id 重写、边映射、未知类型拒绝、悬空边拒绝
- `resolveAIAssistantOperation`：关键词意图判断

外部 LLM 调用通过手工验收：在「设置」tab 配置 API Key → 测试连接 → 在 AI 助手中尝试生成。

## 不做事项

第一版不做：

- MCP 接入
- 工作流生成的流式 token 输出（chat 是流式，generate_workflow 是阶段进度）
- 模型自动执行生成结果（生成完用户必须手动打开画布、确认、运行）
- 自动修复失败工作流
- API Key 加密保存
- 任意自定义 HTTP 模板

## 后续扩展

```
JSON Schema 强约束输出（structured output）
生成失败时模型自我修复一次
画布预览前的 diff 视图
危险节点确认策略
API Key 从环境变量读取
MCP 作为外部工具能力
generate_workflow 的流式进度（边生成边渲染）
```
