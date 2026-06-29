import * as React from 'react';
import { cn } from '../../lib/utils';

type ButtonVariant = 'default' | 'secondary' | 'outline' | 'ghost';
type ButtonSize = 'default' | 'sm' | 'icon';

type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: ButtonVariant;
  size?: ButtonSize;
};

const buttonVariants: Record<ButtonVariant, string> = {
  default: 'inline-flex items-center justify-center rounded-md bg-sky-600 px-4 py-2 text-sm font-semibold text-white hover:bg-sky-700',
  secondary: 'inline-flex items-center justify-center rounded-md bg-slate-100 px-4 py-2 text-sm font-semibold text-slate-900 hover:bg-slate-200',
  outline: 'inline-flex items-center justify-center rounded-md border border-slate-300 bg-white px-4 py-2 text-sm font-semibold text-slate-900 hover:bg-slate-50',
  ghost: 'inline-flex items-center justify-center rounded-md px-3 py-2 text-sm font-semibold text-slate-800 hover:bg-slate-100',
};

const buttonSizes: Record<ButtonSize, string> = {
  default: '',
  sm: 'h-8 px-3',
  icon: 'h-8 w-8 p-0',
};

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(({ className, variant = 'default', size = 'default', ...props }, ref) => {
  return (
    <button
      ref={ref}
      className={cn(buttonVariants[variant], buttonSizes[size], className)}
      {...props}
    />
  );
});

Button.displayName = 'Button';
export { Button };
