// 轻量 Markdown 渲染组件，覆盖 OpsDoc 报告常用的标题/列表/引用/代码块/行内代码/粗体。
// 不引入 react-markdown 等依赖以保持 bundle 体积；如果未来报告需要表格、链接等更多语法，再切换库。

import type { JSX } from 'react';

export function MarkdownView({ body, className }: { body: string; className?: string }) {
  return (
    <div className={`space-y-3 text-sm leading-6 text-slate-800 ${className ?? ''}`.trim()}>
      {renderBlocks(body)}
    </div>
  );
}

// renderBlocks 把 Markdown 切成段落级块（代码块 / 标题 / 列表 / 引用 / 普通段）。
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
        blocks.push(
          <pre
            key={`code-${blocks.length}`}
            className="overflow-auto rounded-md bg-slate-950 px-3 py-2 text-xs leading-5 text-slate-100"
          >
            {codeLang && (
              <div className="mb-1 text-[10px] uppercase text-slate-400">{codeLang}</div>
            )}
            <code>{code.join('\n')}</code>
          </pre>,
        );
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
      const className =
        level === 1
          ? 'border-b border-slate-200 pb-2 text-lg font-semibold text-slate-950'
          : level === 2
            ? 'pt-2 text-base font-semibold text-slate-900'
            : 'text-sm font-semibold text-slate-800';
      blocks.push(
        <div key={`heading-${blocks.length}`} className={className}>
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
          className="rounded-md border-l-4 border-amber-300 bg-amber-50 px-3 py-2 text-amber-900"
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

// renderInline 处理行内代码（`code`）与粗体（**bold**）。
function renderInline(text: string) {
  const parts = text.split(/(`[^`]+`|\*\*[^*]+\*\*)/g);
  return parts.map((part, index) => {
    if (part.startsWith('`') && part.endsWith('`')) {
      return (
        <code
          key={index}
          className="rounded bg-slate-100 px-1 py-0.5 font-mono text-xs text-slate-800"
        >
          {part.slice(1, -1)}
        </code>
      );
    }
    if (part.startsWith('**') && part.endsWith('**')) {
      return (
        <strong key={index} className="font-semibold text-slate-950">
          {part.slice(2, -2)}
        </strong>
      );
    }
    return part;
  });
}
