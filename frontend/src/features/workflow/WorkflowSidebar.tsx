// 工作流左侧栏：变量面板（支持拖入画布生成 var_get 节点）

import type { WorkflowDef, VariableDef } from '@/types/workflow';
import { VariablePanel } from './VariablePanel';

interface Props {
  workflow: WorkflowDef;
  onChange: (workflow: WorkflowDef) => void;
}

export function WorkflowSidebar({ workflow, onChange }: Props) {
  return (
    <aside className="flex h-full w-60 flex-col overflow-y-auto border-r border-slate-200 bg-white">
      <VariablePanel<VariableDef>
        title="Variables"
        items={workflow.variables}
        onChange={(items) => onChange({ ...workflow, variables: items })}
        showDefault
        // 变量拖入画布 → var_get 节点
        dragPayload={(item) => ({
          type_id: 'var_get',
          config: { var_name: item.name, var_type: item.var_type },
        })}
      />
    </aside>
  );
}
