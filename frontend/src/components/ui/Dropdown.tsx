// Dropdown 组件：封装 Radix DropdownMenu，统一暗色菜单样式。
import * as DropdownMenu from '@radix-ui/react-dropdown-menu';
import type { ReactNode } from 'react';
import { cn } from '@/lib/cn';

interface DropdownProps {
  trigger: ReactNode;
  children: ReactNode;
  align?: 'start' | 'center' | 'end';
}

interface DropdownItemProps {
  children: ReactNode;
  disabled?: boolean;
  className?: string;
  onSelect?: () => void;
}

export function Dropdown({ trigger, children, align = 'end' }: DropdownProps) {
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>{trigger}</DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align={align}
          sideOffset={6}
          className={cn(
            'z-50 min-w-52 rounded-md border border-ops-border-subtle bg-ops-elevated p-1 text-sm text-ops-primary shadow-2xl',
            'data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:slide-in-from-top-2 data-[state=open]:duration-base',
            'data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:duration-fast',
          )}
        >
          {children}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

export function DropdownItem({
  children,
  disabled = false,
  className,
  onSelect,
}: DropdownItemProps) {
  return (
    <DropdownMenu.Item
      disabled={disabled}
      className={cn(
        'cursor-default select-none rounded px-2 py-1.5 outline-none transition-colors duration-fast ease-ops',
        'data-[highlighted]:bg-ops-surface data-[highlighted]:text-ops-primary',
        'data-[disabled]:text-ops-tertiary',
        className,
      )}
      onSelect={onSelect}
    >
      {children}
    </DropdownMenu.Item>
  );
}

export function DropdownSeparator() {
  return <DropdownMenu.Separator className="my-1 h-px bg-ops-border-subtle" />;
}
