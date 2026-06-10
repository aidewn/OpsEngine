// Card 组件：提供暗色主题下的基础分区容器。
import type { ReactNode } from 'react';
import { cn } from '@/lib/cn';

interface CardProps {
  children: ReactNode;
  className?: string;
}

export function Card({ children, className }: CardProps) {
  return (
    <section className={cn('rounded-lg border border-ops-border-subtle bg-ops-surface', className)}>
      {children}
    </section>
  );
}

export function CardHeader({ children, className }: CardProps) {
  return <header className={cn('border-b border-ops-border-subtle px-4 py-3', className)}>{children}</header>;
}

export function CardBody({ children, className }: CardProps) {
  return <div className={cn('px-4 py-3', className)}>{children}</div>;
}

export function CardFooter({ children, className }: CardProps) {
  return <footer className={cn('border-t border-ops-border-subtle px-4 py-3', className)}>{children}</footer>;
}
