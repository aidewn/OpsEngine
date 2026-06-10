// 轻量 Markdown 渲染组件，覆盖 OpsDoc 报告常用的标题、列表、引用、代码块、行内代码和粗体。
// 暂不引入 react-markdown，后续需要表格或链接等更多语法时再切换库。
import type { JSX } from 'react';
import { MermaidView } from './MermaidView';

export function MarkdownView({ body, className }: { body: string; className?: string }) {
  return (
    <div className={`space-y-3 text-sm leading-6 text-ops-primary ${className ?? ''}`.trim()}>
      {renderBlocks(body)}
    </div>
  );
}

// renderBlocks 把 Markdown 切成段落级块。
function renderBlocks(body: string) {
  const lines = body.split(/\r?\n/);
  const blocks: JSX.Element[] = [];
  let code: string[] | null = null;
  let codeLang = '';
  let list: string[] = [];

  function flushList() {
    if (list.length === 0) return;
    const items = list;
    list = [];
    blocks.push(
      <ul key={`list-${blocks.length}`} className="list-disc space-y-1 pl-5">
        {items.map((item, index) => (
          <li key={index}>{renderInline(item)}</li>
        ))}
      </ul>,
    );
  }

  for (const line of lines) {
    if (line.startsWith('```')) {
      if (code) {
        const codeBody = code.join('\n');
        if (codeLang.toLowerCase() === 'mermaid') {
          blocks.push(<MermaidView key={`mermaid-${blocks.length}`} source={codeBody} />);
        } else {
          blocks.push(
            <pre
              key={`code-${blocks.length}`}
              className="overflow-auto rounded-md bg-ops-input px-3 py-2 text-xs leading-5 text-ops-primary"
            >
              {codeLang && (
                <div className="mb-1 text-[10px] uppercase text-ops-tertiary">{codeLang}</div>
              )}
              <code>{codeBody}</code>
            </pre>,
          );
        }
        code = null;
        codeLang = '';
      } else {
        flushList();
        code = [];
        codeLang = line.slice(3).trim();
      }
      continue;
    }
    if (code) {
      code.push(line);
      continue;
    }

    const trimmed = line.trim();
    if (!trimmed) {
      flushList();
      continue;
    }
    const heading = trimmed.match(/^(#{1,4})\s+(.+)$/);
    if (heading) {
      flushList();
      const level = heading[1]!.length;
      const text = heading[2]!;
      const headingClassName =
        level === 1
          ? 'border-b border-ops-border-subtle pb-2 text-lg font-semibold text-ops-primary'
          : level === 2
            ? 'pt-2 text-base font-semibold text-ops-primary'
            : 'text-sm font-semibold text-ops-primary';
      blocks.push(
        <div key={`heading-${blocks.length}`} className={headingClassName}>
          {renderInline(text)}
        </div>,
      );
      continue;
    }
    if (trimmed.startsWith('>')) {
      flushList();
      blocks.push(
        <blockquote
          key={`quote-${blocks.length}`}
          className="rounded-md border-l-4 border-ops-warning bg-ops-warning-soft px-3 py-2 text-ops-primary"
        >
          {renderInline(trimmed.replace(/^>\s?/, ''))}
        </blockquote>,
      );
      continue;
    }
    const bullet = trimmed.match(/^[-*]\s+(.+)$/);
    if (bullet) {
      list.push(bullet[1]!);
      continue;
    }
    flushList();
    blocks.push(
      <p key={`p-${blocks.length}`} className="whitespace-pre-wrap">
        {renderInline(trimmed)}
      </p>,
    );
  }
  flushList();
  return blocks;
}

// renderInline 处理行内代码与粗体。
function renderInline(text: string) {
  const parts = text.split(/(`[^`]+`|\*\*[^*]+\*\*)/g);
  return parts.map((part, index) => {
    if (part.startsWith('`') && part.endsWith('`')) {
      return (
        <code
          key={index}
          className="rounded bg-ops-input px-1 py-0.5 font-mono text-xs text-ops-primary"
        >
          {part.slice(1, -1)}
        </code>
      );
    }
    if (part.startsWith('**') && part.endsWith('**')) {
      return (
        <strong key={index} className="font-semibold text-ops-primary">
          {part.slice(2, -2)}
        </strong>
      );
    }
    return part;
  });
}
