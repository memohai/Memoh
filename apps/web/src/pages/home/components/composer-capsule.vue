<template>
  <div
    data-slot="input-group"
    role="group"
    :aria-label="label || undefined"
    class="chat-composer-edge rounded-[20px] bg-surface-composer px-2.5 py-2.5"
  >
    <slot />
  </div>
</template>

<script setup lang="ts">
// ComposerCapsule — the ONE implementation of the composer-dock shell. Every
// box that lives in the dock wears this: the ask_user capsule, the stack-tier
// panel (approvals / command results / errors), and any future dock member.
// The composer itself is the deliberate exception — it animates its radius
// (28px pill ↔ 20px multiline) and drives a height morph, so it keeps its own
// markup; everything else shares this shell so the chrome (input-group edge,
// surface, 20px radius, padding) can never drift into N hand-copies.
defineProps<{
  // Accessible region name; omit when the content already names itself.
  label?: string
}>()
</script>
