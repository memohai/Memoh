<template>
  <svg
    xmlns="http://www.w3.org/2000/svg"
    :width="size"
    :height="size"
    viewBox="0 0 1024 1024"
    v-bind="$attrs"
  ><path
    d="M512 1024c282.781538 0 512-229.218462 512-512S794.781538 0 512 0 0 229.218462 0 512s229.218462 512 512 512z"
    fill="#FC3F1D"
    p-id="3294"
  /><path
    d="M526.966154 569.619692c31.980308 68.253538 40.487385 91.726769 40.487384 174.907077v108.819693h-110.907076v-183.492924L247.453538 213.346462h115.2l164.273231 356.27323z m136.507077-356.27323l-136.546462 309.326769h113.073231l136.546462-309.326769h-113.073231z"
    fill="#FFFFFF"
    p-id="3295"
  /></svg>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ size?: string | number }>(), { size: '1em' })
defineOptions({ inheritAttrs: false })
</script>
