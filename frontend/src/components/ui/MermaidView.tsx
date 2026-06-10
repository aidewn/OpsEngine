// MermaidView 组件：懒加载 mermaid 并渲染架构图。
import { useEffect, useId, useState } from 'react';
import { Dialog } from '@/components/ui/Dialog';

interface MermaidViewProps {
  source: string;
}

export function MermaidView({ source }: MermaidViewProps) {
  const reactId = useId();
  const diagramID = `mermaid-${reactId.replace(/[^a-zA-Z0-9_-]/g, '')}`;
  const [svg, setSvg] = useState('');
  const [error, setError] = useState('');
  const [fullscreen, setFullscreen] = useState(false);

  useEffect(() => {
    let cancelled = false;

    async function renderDiagram() {
      try {
        setError('');
        const mermaid = (await import('mermaid')).default;
        mermaid.initialize({
          startOnLoad: false,
          theme: 'dark',
          securityLevel: 'strict',
          themeVariables: {
            background: '#1A1A19',
            primaryColor: '#262624',
            primaryTextColor: '#F5F4ED',
            lineColor: '#A8A6A1',
            secondaryColor: '#2F2F2C',
            tertiaryColor: '#1F1F1D',
          },
        });
        const result = await mermaid.render(diagramID, source);
        if (!cancelled) setSvg(result.svg);
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Mermaid 渲染失败');
      }
    }

    renderDiagram();
    return () => {
      cancelled = true;
    };
  }, [diagramID, source]);

  if (error) {
    return (
      <pre className="overflow-auto rounded-md border border-ops-danger bg-ops-danger-soft px-3 py-2 text-xs text-ops-primary">
        {error}
      </pre>
    );
  }

  if (!svg) {
    return <div className="rounded-md bg-ops-input px-3 py-4 text-sm text-ops-secondary">图表渲染中…</div>;
  }

  return (
    <>
      <button
        type="button"
        className="block w-full overflow-auto rounded-md border border-ops-border-subtle bg-ops-input p-3 text-left"
        title="点击全屏查看"
        onClick={() => setFullscreen(true)}
      >
        <div className="min-w-fit" dangerouslySetInnerHTML={{ __html: svg }} />
      </button>
      <Dialog
        open={fullscreen}
        onOpenChange={setFullscreen}
        title="Mermaid 图表"
        size="fullscreen"
        contentClassName="overflow-auto"
      >
        <div className="min-w-fit" dangerouslySetInnerHTML={{ __html: svg }} />
      </Dialog>
    </>
  );
}
