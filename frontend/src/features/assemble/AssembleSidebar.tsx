// 集合左侧栏：Params + Returns + Variables 三个面板

import type { AssembleDef, ParamDef } from '@/types/assemble';
import type { VariableDef } from '@/types/workflow';
import { VariablePanel } from '@/features/workflow/VariablePanel';

interface Props {
  assemble: AssembleDef;
  onChange: (assemble: AssembleDef) => void;
}

export function AssembleSidebar({ assemble, onChange }: Props) {
  return (
    <aside className="flex h-full w-60 flex-col overflow-y-auto border-r border-slate-200 bg-white">
      <VariablePanel<ParamDef>
        title="Params"
        items={assemble.params}
        onChange={(items) => onChange({ ...assemble, params: items })}
        // 参数拖入画布 → assemble_param 节点
        dragPayload={(item) => ({
          type_id: 'assemble_param',
          config: { param_name: item.name, var_type: item.var_type },
        })}
      />
      <VariablePanel<ParamDef>
        title="Returns"
        items={assemble.returns}
        onChange={(items) => onChange({ ...assemble, returns: items })}
        // 返回值暂不支持拖入（设值方式后续设计 set_return 节点）
      />
      <VariablePanel<VariableDef>
        title="Variables"
        items={assemble.variables}
        onChange={(items) => onChange({ ...assemble, variables: items })}
        showDefault
        dragPayload={(item) => ({
          type_id: 'var_get',
          config: { var_name: item.name, var_type: item.var_type },
        })}
      />
    </aside>
  );
}
