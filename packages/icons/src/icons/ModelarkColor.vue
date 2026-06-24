<template>
  <svg
    xmlns="http://www.w3.org/2000/svg"
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    v-bind="$attrs"
  ><defs><linearGradient
    :id="`${uid}-lobe-icons-modelark-fill`"
    x1="2"
    x2="22"
    y1="3"
    y2="21"
    gradientUnits="userSpaceOnUse"
  ><stop stop-color="#00D6FF" /><stop
    offset=".52"
    stop-color="#2F6BFF"
  /><stop
    offset="1"
    stop-color="#7B3DFF"
  /></linearGradient></defs><path
    d="M3 6.4C3 4.522 4.522 3 6.4 3h11.2C19.478 3 21 4.522 21 6.4v11.2c0 1.878-1.522 3.4-3.4 3.4H6.4A3.4 3.4 0 0 1 3 17.6V6.4z"
    :fill="`url(#${uid}-lobe-icons-modelark-fill)`"
  /><path
    d="M6.2 17.25V6.75h2.6L12 11.2l3.2-4.45h2.6v10.5h-2.75v-5.8L12 15.55l-3.05-4.1v5.8H6.2z"
    fill="#fff"
  /></svg>
</template>

<script setup lang="ts">
import { useId } from 'vue'
withDefaults(defineProps<{ size?: string | number }>(), { size: '1em' })
defineOptions({ inheritAttrs: false })

const uid = useId()
</script>
