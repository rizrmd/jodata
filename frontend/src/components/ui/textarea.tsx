import * as React from 'react';
import { cn } from '../../lib/utils';

const Textarea = React.forwardRef<HTMLTextAreaElement, React.TextareaHTMLAttributes<HTMLTextAreaElement>>(({ className, ...props }, ref) => (
  <textarea
    ref={ref}
    className={cn('min-h-[80px] w-full rounded-md border border-slate-200 bg-white px-3 py-2 text-sm outline-none transition-colors focus:border-sky-500 focus:ring-2 focus:ring-sky-200', className)}
    {...props}
  />
));

Textarea.displayName = 'Textarea';
export { Textarea };
