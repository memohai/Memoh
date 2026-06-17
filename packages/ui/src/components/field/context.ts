import type { ComputedRef, InjectionKey, Ref } from 'vue'

// Shared wiring for a single field (label ↔ control ↔ description ↔ error).
// Provided by <Field>, consumed by FieldLabel / FieldControl / FieldDescription
// / FieldError so ids and aria-* link up automatically without manual props.
export interface FieldContext {
  id: string
  descriptionId: string
  errorId: string
  invalid: ComputedRef<boolean>
  describedBy: ComputedRef<string | undefined>
  hasDescription: Ref<boolean>
  hasError: Ref<boolean>
}

export const FIELD_INJECTION_KEY: InjectionKey<FieldContext> = Symbol('memoh-field')
