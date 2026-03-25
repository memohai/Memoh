import {onActivated, reactive, useId } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'

export default () => {
  const visibleStatus = reactive({
    leftDefault: '.main-left-section',
    rightDefault:'.main-right-section',
    id:useId()
  })
  
  onBeforeRouteLeave(() => {
    visibleStatus.leftDefault = `.${visibleStatus.id}`
    visibleStatus.rightDefault=`.${visibleStatus.id}`
  })

  onActivated(() => {
    visibleStatus.leftDefault = '.main-left-section'
    visibleStatus.rightDefault='.main-right-section'
  })

  return visibleStatus
}