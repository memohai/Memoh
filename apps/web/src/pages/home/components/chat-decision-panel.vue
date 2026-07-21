<template>
  <div
    data-slot="input-group"
    role="group"
    class="chat-composer-edge chat-composer-radius-multiline flex w-full flex-col bg-surface-composer px-2.5 py-2.5"
  >
    <div class="max-h-[40vh] overflow-y-auto overscroll-contain [scrollbar-gutter:stable]">
      <slot />
    </div>
    <div class="mt-2 border-t border-border-soft pt-2">
      <div class="flex flex-wrap gap-1.5">
        <slot name="actions" />
      </div>
    </div>
  </div>
</template>
