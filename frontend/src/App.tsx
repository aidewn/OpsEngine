// 路由配置

import { Navigate, Route, Routes } from 'react-router-dom';
import { HomePage } from '@/pages/HomePage';
import { WorkflowCanvasPage } from '@/pages/WorkflowCanvasPage';
import { AssembleCanvasPage } from '@/pages/AssembleCanvasPage';
import { ExecutionDetailPage } from '@/pages/ExecutionDetailPage';

export function App() {
  return (
    <Routes>
      <Route path="/" element={<HomePage />} />
      <Route path="/workflows/:id" element={<WorkflowCanvasPage />} />
      <Route path="/assembles/:id" element={<AssembleCanvasPage />} />
      <Route path="/executions/:id" element={<ExecutionDetailPage />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
