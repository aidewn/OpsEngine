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
        // 参数拖入画布 → Get 参数（assemble_param），与 Variables 拖 var_get 对称
        dragPayload={(item) => ({
          type_id: 'assemble_param',
          config: { param_name: item.name, var_type: item.var_type },
        })}
      />
      <VariablePanel<ParamDef>
        title="Returns"
        items={assemble.returns}
        onChange={(items) => onChange({ ...assemble, returns: items })}
        // 返回值拖入画布 → Set 返回值（return_set），与 Variables 拖 var_set 对称
        dragPayload={(item) => ({
          type_id: 'return_set',
          config: { return_name: item.name, var_type: item.var_type },
        })}
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
