<template>
  <!-- wizard 步骤的退出动画壳,唯一 owner。抽出前同一段 scale+opacity 过渡在
       step-frame / Step1 / Step3 外层 / Step5 共四处手抄,改时长必漏改。
       每个 Step*.vue 的模板根都应包在这个壳里;StepFrame 不再拥有退出动画
       (Step3 一页里有三个 StepFrame 实例共用一个壳,壳只能在 frame 之上)。 -->
  <div
    class="transition-all duration-[175ms] ease-out"
    :class="exiting ? 'scale-[0.88] opacity-0' : 'scale-100 opacity-100'"
  >
    <slot />
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  exiting?: boolean
}>(), {
  exiting: false,
})
</script>
