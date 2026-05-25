// 集合左侧栏：Params + Returns + Variables 三个面板

import type { ParamDef } from '@/types/assemble';
import type { VariableDef } from '@/types/workflow';
import { VariablePanel } from '@/features/workflow/VariablePanel';

interface Props {
  onParamsChange: (items: ParamDef[]) => void;
  onReturnsChange: (items: ParamDef[]) => void;
  onVariablesChange: (items: VariableDef[]) => void;
  params: ParamDef[];
  returns: ParamDef[];
  variables: VariableDef[];
}

export function AssembleSidebar({
  params,
  returns,
  variables,
  onParamsChange,
  onReturnsChange,
  onVariablesChange,
}: Props) {
  return (
    <aside className="flex h-full w-60 flex-col overflow-y-auto border-r border-slate-200 bg-white">
      <VariablePanel<ParamDef>
        title="Params"
        items={params}
        onChange={onParamsChange}
        // 参数拖入画布 → Get 参数（assemble_param），与 Variables 拖 var_get 对称
        dragPayload={(item) => ({
          type_id: 'assemble_param',
          config: { param_name: item.name, var_type: item.var_type },
        })}
      />
      <VariablePanel<ParamDef>
        title="Returns"
        items={returns}
        onChange={onReturnsChange}
        // 返回值拖入画布 → Set 返回值（return_set），与 Variables 拖 var_set 对称
        dragPayload={(item) => ({
          type_id: 'return_set',
          config: { return_name: item.name, var_type: item.var_type },
        })}
      />
      <VariablePanel<VariableDef>
        title="Variables"
        items={variables}
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
