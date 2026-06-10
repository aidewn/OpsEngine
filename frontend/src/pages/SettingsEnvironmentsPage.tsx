// SettingsEnvironmentsPage 组件：标题栏设置入口打开的环境配置全屏视图。
import { useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui/Button';
import { EnvironmentList } from '@/features/environment/EnvironmentList';

export function SettingsEnvironmentsPage() {
  const navigate = useNavigate();

  return (
    <div className="h-full overflow-auto bg-ops-canvas px-6 py-6 text-ops-primary">
      <div className="mx-auto max-w-5xl">
        <div className="mb-5 flex items-center justify-between">
          <div>
            <h1 className="text-xl font-semibold">环境配置</h1>
            <p className="mt-1 text-sm text-ops-secondary">集中管理 SSH、Docker、K8s 和 Jenkins 凭证。</p>
          </div>
          {/* 这里固定回首页，而不是 navigate(-1)。
              原因：用户在本页点入"环境详情"→ 详情页"返回环境列表"是 Link push（又压一层），
              导致历史栈像 / → /settings/environments → /settings/environments/:id → /settings/environments，
              此时 navigate(-1) 会回到详情页形成死循环。 */}
          <Button variant="secondary" onClick={() => navigate('/')}>
            返回首页
          </Button>
        </div>
        <EnvironmentList detailPathPrefix="/settings/environments" />
      </div>
    </div>
  );
}
