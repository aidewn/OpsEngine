// HostStatusCard：ssh_inspect 的主机状态仪表卡。
// 三个环形仪表（CPU/内存/最满磁盘）+ 磁盘分区横条 + Top 进程小表。
// 动效：仪表首帧从 0 充能到目标值（SVG stroke-dashoffset 过渡），>85% 用 danger 色。
import { useEffect, useState } from 'react';
import { RefreshCw } from 'lucide-react';
import { cn } from '@/lib/cn';
import { useViewActions } from './ViewActions';

// 与后端 builtin.hostStatusData 对齐
interface HostStatusData {
  hostname?: string;
  os?: string;
  uptime_sec?: number;
  cores?: number;
  load1?: number;
  cpu_percent?: number;
  mem_total?: number;
  mem_used?: number;
  disks?: { mount: string; total: number; used: number }[];
  procs?: { pid: string; user: string; cpu: number; mem: number; cmd: string }[];
}

export function HostStatusCard({ title, data }: { title: string; data: HostStatusData }) {
  const { resend } = useViewActions();
  const memPct = pct(data.mem_used, data.mem_total);
  const fullestDisk = (data.disks ?? []).reduce<{ mount: string; p: number } | null>(
    (acc, d) => {
      const p = pct(d.used, d.total);
      return p !== null && (!acc || p > acc.p) ? { mount: d.mount, p } : acc;
    },
    null,
  );
  const cpuPct = data.cpu_percent != null && data.cpu_percent >= 0 ? data.cpu_percent : null;

  return (
    <div className="rounded-md border border-ops-border-subtle bg-ops-input p-3">
      <div className="mb-3 flex items-baseline justify-between gap-2">
        <div className="min-w-0">
          <div className="truncate text-xs font-medium text-ops-primary">{title}</div>
          {data.os ? <div className="truncate text-2xs text-ops-tertiary">{data.os}</div> : null}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {data.uptime_sec ? (
            <span className="font-mono text-2xs tabular-nums text-ops-tertiary">
              up {formatUptime(data.uptime_sec)}
            </span>
          ) : null}
          {resend ? (
            <button
              type="button"
              onClick={() => resend('重新采集这台机器的状态')}
              className="rounded p-1 text-ops-tertiary transition-colors duration-fast ease-ops hover:bg-ops-surface hover:text-ops-primary"
              title="重新采集（快照）"
              aria-label="重新采集"
            >
              <RefreshCw size={12} />
            </button>
          ) : null}
        </div>
      </div>

      <div className="flex items-start justify-around gap-2">
        <Gauge label="CPU" percent={cpuPct} sub={data.cores ? `${data.cores} 核` : undefined} />
        <Gauge label="内存" percent={memPct} sub={formatBytes(data.mem_total)} />
        <Gauge label="磁盘" percent={fullestDisk?.p ?? null} sub={fullestDisk?.mount} />
      </div>

      {data.disks && data.disks.length > 0 && (
        <div className="mt-3 space-y-1.5">
          {data.disks.slice(0, 4).map((d) => {
            const p = pct(d.used, d.total) ?? 0;
            return (
              <div key={d.mount} className="flex items-center gap-2 text-2xs">
                <span className="w-20 truncate font-mono text-ops-secondary">{d.mount}</span>
                <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-ops-surface">
                  <div
                    className={cn(
                      'h-full rounded-full transition-[width] duration-slow ease-ops',
                      p > 85 ? 'bg-ops-danger' : p > 70 ? 'bg-ops-warning' : 'bg-ops-info',
                    )}
                    style={{ width: `${p}%` }}
                  />
                </div>
                <span className="w-20 shrink-0 text-right font-mono tabular-nums text-ops-tertiary">
                  {formatBytes(d.used)}/{formatBytes(d.total)}
                </span>
              </div>
            );
          })}
        </div>
      )}

      {data.procs && data.procs.length > 0 && (
        <div className="mt-3 border-t border-ops-border-subtle pt-2">
          <div className="mb-1 text-2xs text-ops-tertiary">CPU Top 进程</div>
          <div className="space-y-0.5 font-mono text-2xs">
            {data.procs.slice(0, 5).map((p) => (
              <div key={p.pid} className="flex items-center gap-2">
                <span className="w-12 tabular-nums text-ops-tertiary">{p.pid}</span>
                <span className="w-16 truncate text-ops-secondary">{p.user}</span>
                <span className="w-12 text-right tabular-nums text-ops-secondary">{p.cpu.toFixed(1)}%</span>
                <span className="min-w-0 flex-1 truncate text-ops-primary">{p.cmd}</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// Gauge 环形仪表：挂载后从 0 充能到目标值；percent 为 null 时显示占位。
function Gauge({ label, percent, sub }: { label: string; percent: number | null; sub?: string }) {
  const [value, setValue] = useState(0);
  useEffect(() => {
    if (percent === null) return;
    // 下一帧再设目标值，触发 stroke-dashoffset 过渡（充能动画）
    const t = window.setTimeout(() => setValue(percent), 30);
    return () => window.clearTimeout(t);
  }, [percent]);

  const r = 26;
  const c = 2 * Math.PI * r;
  const danger = percent !== null && percent > 85;
  const warn = percent !== null && percent > 70 && percent <= 85;
  return (
    <div className="flex flex-col items-center gap-0.5">
      <svg width="72" height="72" viewBox="0 0 72 72" aria-hidden>
        <circle cx="36" cy="36" r={r} fill="none" stroke="#333331" strokeWidth="6" />
        {percent !== null && (
          <circle
            cx="36"
            cy="36"
            r={r}
            fill="none"
            stroke={danger ? '#EF4444' : warn ? '#F59E0B' : '#60A5FA'}
            strokeWidth="6"
            strokeLinecap="round"
            strokeDasharray={c}
            strokeDashoffset={c - (c * Math.min(value, 100)) / 100}
            transform="rotate(-90 36 36)"
            style={{ transition: 'stroke-dashoffset 600ms cubic-bezier(0.22, 1, 0.36, 1)' }}
          />
        )}
        <text
          x="36"
          y="40"
          textAnchor="middle"
          className="fill-ops-primary font-mono text-sm tabular-nums"
        >
          {percent === null ? '—' : `${Math.round(value)}%`}
        </text>
      </svg>
      <span className="text-2xs text-ops-secondary">{label}</span>
      {sub ? <span className="max-w-20 truncate font-mono text-2xs text-ops-tertiary">{sub}</span> : null}
    </div>
  );
}

// pct 计算百分比，无效输入返回 null。
function pct(used?: number, total?: number): number | null {
  if (!used || !total || total <= 0) return null;
  return Math.min(100, Math.max(0, (used / total) * 100));
}

// formatBytes 人类可读容量。
function formatBytes(n?: number): string {
  if (!n || n <= 0) return '—';
  const units = ['B', 'K', 'M', 'G', 'T'];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v >= 10 ? Math.round(v) : v.toFixed(1)}${units[i]}`;
}

// formatUptime 把秒转为 "3d 4h" / "5h 12m"。
function formatUptime(sec: number): string {
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}
