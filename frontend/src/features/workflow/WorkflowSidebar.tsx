// 工作流左侧栏：变量面板（支持拖入画布生成 var_get 节点）

import type { WorkflowDef, VariableDef } from '@/types/workflow';
import { VariablePanel } from './VariablePanel';

interface Props {
  workflow: WorkflowDef;
  onVariablesChange: (items: VariableDef[]) => void;
}

export function WorkflowSidebar({ workflow, onVariablesChange }: Props) {
  return (
    <aside className="flex h-full w-60 flex-col overflow-y-auto border-r border-slate-200 bg-white">
      <VariablePanel<VariableDef>
        title="Variables"
        items={workflow.variables ?? []}
        onChange={onVariablesChange}
        showDefault
        // 整行拖入画布 → var_get；行内 Set chip 拖入 → var_set
        dragPayload={(item) => ({
          type_id: 'var_get',
          config: { var_name: item.name, var_type: item.var_type },
        })}
        secondaryDragPayload={(item) => ({
          type_id: 'var_set',
          config: { var_name: item.name, var_type: item.var_type },
        })}
        secondaryLabel="Set"
      />
    </aside>
  );
}
