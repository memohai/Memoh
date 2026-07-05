<template>
  <!-- 这是 wizard 步骤帧的 owner;之前 Step2/3/4 各手写一份,盒内 pt 漂移
       (none/pt-24/pt-16) 导致切步时标题跳动;owner 统一为无 pt + mb-6,标题在
       每一步同一 y。 -->
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
        class="text-3xl font-semibold mb-6 transition-all duration-[350ms] ease-out"
        :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
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
  bodyClass: undefined,
})
</script>
