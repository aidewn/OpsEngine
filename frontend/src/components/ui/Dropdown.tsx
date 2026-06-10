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
          className="z-50 min-w-52 rounded-md border border-ops-border-subtle bg-ops-elevated p-1 text-sm text-ops-primary shadow-2xl"
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
        'cursor-default select-none rounded px-2 py-1.5 outline-none transition-colors',
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
