// JobListCard：jenkins_list_jobs 视图。Job 名 + 可点开的链接。
import { useState } from 'react';
import { ExternalLink } from 'lucide-react';
import { StatusDot, ViewCard } from './StatusDot';
import { Empty, MoreToggle } from './ContainerListCard';

interface Job {
  name: string;
  url?: string;
}

// openExternal 用 Wails BrowserOpenURL 在系统默认浏览器打开（内网 Jenkins 链接常见）。
function openExternal(url: string) {
  const rt = (window as Window & { runtime?: { BrowserOpenURL?: (u: string) => void } }).runtime;
  rt?.BrowserOpenURL?.(url);
}

const COLLAPSE_AT = 10;

export function JobListCard({ title, data }: { title: string; data: Job[] }) {
  const [expanded, setExpanded] = useState(false);
  if (data.length === 0) return <ViewCard title={title} count={0}><Empty /></ViewCard>;
  const shown = expanded ? data : data.slice(0, COLLAPSE_AT);
  return (
    <ViewCard title={title} count={data.length}>
      <div className="space-y-0.5 text-2xs">
        {shown.map((j, i) => (
          <div key={`${j.name}/${i}`} className="flex items-center gap-2">
            <StatusDot tone="muted" />
            <span className="min-w-0 flex-1 truncate font-mono text-ops-primary">{j.name}</span>
            {j.url ? (
              <button
                type="button"
                onClick={() => openExternal(j.url!)}
                className="shrink-0 text-ops-tertiary transition-colors duration-fast ease-ops hover:text-ops-info"
                aria-label="打开 Job"
                title="在浏览器打开"
              >
                <ExternalLink size={11} />
              </button>
            ) : null}
          </div>
        ))}
      </div>
      {data.length > COLLAPSE_AT && <MoreToggle expanded={expanded} total={data.length} onToggle={() => setExpanded((v) => !v)} />}
    </ViewCard>
  );
}
