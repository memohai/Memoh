<template>
  <!-- wizard 步骤帧的 owner:退出动画壳 + 35rem 盒 + 标题结构。
       各步的盒顶 pt 与标题 mb 是逐页单独调过的视觉位置(不是漂移),
       由调用方经 bodyClass / titleClass 传入,owner 不强行统一。 -->
  <div
    class="transition-all duration-[175ms] ease-out"
    :class="exiting ? 'scale-[0.88] opacity-0' : 'scale-100 opacity-100'"
  >
    <div
      class="text-left h-[35rem] max-h-[calc(100dvh-7rem)] flex flex-col"
      :class="bodyClass"
    >
      <h2
        v-if="title"
        class="text-3xl font-semibold transition-all duration-[350ms] ease-out"
        :class="[titleClass, visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3']"
      >
        {{ title }}
      </h2>
      <slot name="header" />
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  title?: string
  visible?: boolean
  exiting?: boolean
  // 标题下边距,逐页单独调的值(Step2 mb-8 / Step3 mb-3 / Step4 mb-6)
  titleClass?: string
  // Step3 renders three mode variants (list/form/acp), each with its own
  // scale-fade transition on the body div (a second animation layer on top
  // of the exit shell above). Passed through as-is (array/object/string, same
  // as a native :class binding) so the caller's literal Tailwind classes stay
  // scannable — never built by string concatenation.
  bodyClass?: unknown
}>(), {
  title: '',
  visible: false,
  exiting: false,
  titleClass: 'mb-6',
  bodyClass: undefined,
})
</script>
