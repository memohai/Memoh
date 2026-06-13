export { default as SegmentedControl } from './SegmentedControl.vue'

export interface SegmentedItem<V extends string | number = string> {
  value: V
  label?: string
  disabled?: boolean
}
