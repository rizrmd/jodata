import * as React from 'react';

const Label = React.forwardRef<HTMLLabelElement, React.LabelHTMLAttributes<HTMLLabelElement>>(({ className, ...props }, ref) => (
  <label
    ref={ref}
    className={['text-sm font-medium text-slate-700', className].filter(Boolean).join(' ')}
    {...props}
  />
));

Label.displayName = 'Label';
export { Label };
