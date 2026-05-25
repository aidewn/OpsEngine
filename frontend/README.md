# OpsEngine 前端

本目录为 OpsEngine 桌面应用的 Web 前端（Vite + React 18 + TypeScript + React Flow + Tailwind）。

**安装、运行、架构与贡献说明请参阅仓库根目录 [README.md](../README.md)。**

日常开发请在上级目录执行 `make dev` 或 `wails dev`，由 Wails 统一管理前后端热重载。

仅单独调试前端样式时：

```bash
npm install
npm run dev
```

此时 Wails 生成的 `wailsjs` 绑定需在完整桌面环境中才可用。
