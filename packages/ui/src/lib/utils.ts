import type { ClassValue } from 'clsx'
import { clsx } from 'clsx'
import { extendTailwindMerge } from 'tailwind-merge'

// Register our semantic font-size tokens so tailwind-merge treats them as
// font sizes (the `font-size` group) instead of misclassifying `text-<name>`
// as a text color. Without this, a class like `text-control` collides with a
// real color class (e.g. `text-background`) and gets silently dropped.
const twMerge = extendTailwindMerge({
  extend: {
    classGroups: {
      'font-size': [
        {
          text: [
            'caption',
            'body',
            'label',
            'control',
            'title',
            'heading',
            'display',
          ],
        },
      ],
    },
  },
})

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
