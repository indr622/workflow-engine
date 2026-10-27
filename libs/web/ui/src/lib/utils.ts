import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

/** cn menggabungkan class Tailwind dan menyelesaikan konflik antar utility. */
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}
