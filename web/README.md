# OpsEngine Web

OpsEngine 前端：Vite + React 18 + TypeScript + React Flow + Tailwind v3 + TanStack Query。

## 启动

```bash
cd web
npm install
npm run dev        # http://localhost:5173
```

Vite 已配置 `/api` 和 `/ws` 代理到 `http://localhost:8080`，无需后端开 CORS。

## 后端 API 契约（MVP）

前端依赖后端实现以下接口：

| Method | Path | Request | Response | 说明 |
|--------|------|---------|----------|------|
| GET | `/api/workflows` | - | `WorkflowSummary[]` | 列表 |
| POST | `/api/workflows` | `{ name, description? }` | `{ id }` | 创建空工作流，后端生成 id |
| GET | `/api/workflows/:id` | - | `WorkflowDef` | 详情 |
| PUT | `/api/workflows/:id` | `WorkflowDef` | 204 | 整体覆盖更新 |
| GET | `/api/node-types` | - | `NodeTypeDef[]` | 所有已注册节点类型 |

错误响应建议：`{ "error": "<message>" }` + 非 2xx 状态码。

字段命名遵循后端 Go JSON tag（snake_case）。完整类型定义见 `src/types/`。

## 创建工作流的前后端协作

1. 用户点击"新建工作流" → `POST /api/workflows`，后端返回 `{ id }`
2. 前端调 `PUT /api/workflows/:id`，在 `nodes` 字段写入 3 个事件源节点
3. 跳转到 `/workflows/:id`，画布从 `GET /api/workflows/:id` 拉数据渲染

这是纯前端便利约定，后端不需要在创建时插入任何默认节点。

## 目录结构

```
src/
├── main.tsx                          # 入口：QueryClient + Router
├── App.tsx                           # 路由配置
├── index.css                         # Tailwind 入口
│
├── types/                            # 与后端结构对齐的类型
│   ├── workflow.ts                   # WorkflowDef / NodeInstance / EdgeConfig
│   └── nodeType.ts                   # NodeTypeDef + 系统节点 TypeID 常量
│
├── lib/                              # 通用工具
│   ├── cn.ts                         # tailwind class 合并
│   └── uuid.ts                       # crypto.randomUUID 封装
│
├── api/                              # REST 客户端 + TanStack Query hooks
│   ├── client.ts                     # fetch 包装
│   ├── workflows.ts                  # useWorkflows / useWorkflow / useCreateWorkflow / useUpdateWorkflow
│   └── nodeTypes.ts                  # useNodeTypes
│
├── components/ui/                    # 基础 UI 组件（无业务逻辑）
│   ├── Button.tsx
│   ├── Input.tsx
│   ├── Textarea.tsx
│   ├── Label.tsx
│   └── Dialog.tsx                    # 基于 Radix Dialog
│
├── features/workflow/                # 工作流领域组件
│   ├── CreateWorkflowDialog.tsx      # 新建弹窗
│   ├── WorkflowCanvas.tsx            # React Flow 画布
│   ├── NodeDetailPanel.tsx           # 右侧详情面板
│   ├── systemNodes.ts                # 默认 3 事件源节点生成
│   ├── canvasMapping.ts              # WorkflowDef ↔ RF Node/Edge 转换
│   └── nodes/                        # 各类节点的画布渲染组件
│       ├── BaseNode.tsx              # 节点卡片骨架
│       ├── SystemReadyNode.tsx
│       ├── SystemUpdateNode.tsx
│       ├── SystemOverNode.tsx
│       ├── GenericNode.tsx           # 兜底业务节点
│       └── nodeTypeMap.ts            # RF nodeTypes 映射
│
└── pages/                            # 页面组件（路由级）
    ├── WorkflowListPage.tsx
    └── WorkflowCanvasPage.tsx
```

## MVP 范围

- ✅ 工作流列表 + 创建
- ✅ 画布渲染 3 个系统事件源节点
- ✅ 点击节点显示右侧详情，未选中时显示工作流详情
- ✅ 拖动节点自动保存 position
- ❌ 节点 config 表单（下个迭代）
- ❌ 新增/删除节点、连线编辑（下个迭代）
- ❌ WebSocket 实时状态（执行引擎完成后）
