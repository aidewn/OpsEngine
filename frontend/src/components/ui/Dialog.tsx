// 基于 Radix Dialog 的模态框封装
// 仅暴露最常用的 API，复杂场景再扩展

import * as RadixDialog from '@radix-ui/react-dialog';
import { type ReactNode } from 'react';
import { cn } from '@/lib/cn';

interface DialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  children: ReactNode;
  footer?: ReactNode;
}

export function Dialog({
  open,
  onOpenChange,
  title,
  description,
  children,
  footer,
}: DialogProps) {
  return (
    <RadixDialog.Root open={open} onOpenChange={onOpenChange}>
      <RadixDialog.Portal>
        <RadixDialog.Overlay
          className={cn(
            'fixed inset-0 z-40 bg-black/40',
            'data-[state=open]:animate-in data-[state=open]:fade-in-0',
          )}
        />
        <RadixDialog.Content
          className={cn(
            'fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2',
            'rounded-lg bg-white p-6 shadow-xl',
            'focus:outline-none',
          )}
        >
          <RadixDialog.Title className="text-lg font-semibold text-slate-900">
            {title}
          </RadixDialog.Title>
          {description && (
            <RadixDialog.Description className="mt-1 text-sm text-slate-500">
              {description}
            </RadixDialog.Description>
          )}
          <div className="mt-4">{children}</div>
          {footer && (
            <div className="mt-6 flex justify-end gap-2">{footer}</div>
          )}
        </RadixDialog.Content>
      </RadixDialog.Portal>
    </RadixDialog.Root>
  );
}
