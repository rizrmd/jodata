import * as React from 'react';
import { cn } from '../../lib/utils';

const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(({ className, ...props }, ref) => (
  <input
    ref={ref}
    className={cn('h-10 w-full rounded-md border border-slate-200 bg-white px-3 text-sm outline-none transition-colors focus:border-sky-500 focus:ring-2 focus:ring-sky-200', className)}
    {...props}
  />
));

Input.displayName = 'Input';
export { Input };
