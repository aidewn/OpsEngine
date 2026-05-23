import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

// class 合并工具（兼容 tailwind 冲突解析）
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}
