// Minimal type stub for @memohai/ui consumed directly by the desktop
// renderer's own files. @memohai/ui ships a barrel `index.ts` that imports
// every component; pulling those into desktop's typecheck program surfaces
// pre-existing strict-template warnings unrelated to desktop. Routing the
// typecheck through this stub keeps the desktop's surface small.
//
// Vite ignores `paths` and resolves the real `@memohai/ui` package at bundle
// time, so runtime behavior is unchanged.

declare module '@memohai/ui' {
  import type { DefineComponent } from 'vue'

  type LooseComponent = DefineComponent<
    Record<string, unknown>,
    Record<string, unknown>,
    unknown
  >

  export const Toaster: LooseComponent
  export const SidebarInset: LooseComponent
  export const Avatar: LooseComponent
  export const AvatarFallback: LooseComponent
  export const AvatarImage: LooseComponent
  export const Badge: LooseComponent
  export const Breadcrumb: LooseComponent
  export const BreadcrumbItem: LooseComponent
  export const BreadcrumbLink: LooseComponent
  export const BreadcrumbList: LooseComponent
  export const BreadcrumbPage: LooseComponent
  export const BreadcrumbSeparator: LooseComponent
  export const Button: LooseComponent
  export const Card: LooseComponent
  export const CardHeader: LooseComponent
  export const CardTitle: LooseComponent
  export const Dialog: LooseComponent
  export const DialogClose: LooseComponent
  export const DialogContent: LooseComponent
  export const DialogDescription: LooseComponent
  export const DialogFooter: LooseComponent
  export const DialogHeader: LooseComponent
  export const DialogTitle: LooseComponent
  export const DialogTrigger: LooseComponent
  export const Empty: LooseComponent
  export const EmptyContent: LooseComponent
  export const EmptyDescription: LooseComponent
  export const EmptyHeader: LooseComponent
  export const EmptyMedia: LooseComponent
  export const EmptyTitle: LooseComponent
  export const FormControl: LooseComponent
  export const FormField: LooseComponent
  export const FormItem: LooseComponent
  export const Input: LooseComponent
  export const Label: LooseComponent
  export const Select: LooseComponent
  export const SelectContent: LooseComponent
  export const SelectItem: LooseComponent
  export const SelectTrigger: LooseComponent
  export const SelectValue: LooseComponent
  export const Separator: LooseComponent
  export const Spinner: LooseComponent
  export const Tooltip: LooseComponent
  export const TooltipContent: LooseComponent
  export const TooltipTrigger: LooseComponent
}
