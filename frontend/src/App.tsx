// 路由配置

import { Navigate, Route, Routes } from 'react-router-dom';
import { WorkflowListPage } from '@/pages/WorkflowListPage';
import { WorkflowCanvasPage } from '@/pages/WorkflowCanvasPage';

export function App() {
  return (
    <Routes>
      <Route path="/" element={<WorkflowListPage />} />
      <Route path="/workflows/:id" element={<WorkflowCanvasPage />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
