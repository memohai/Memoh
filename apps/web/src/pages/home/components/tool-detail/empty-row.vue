<template>
  <!--
    Owner for the "this tool result has nothing to show" row that ~20 detail
    panels each hand-rolled with drifting classes (some inherited `text-xs`
    from a parent, others repeated it, `italic` always present but at a
    different specificity depending on parent wrappers). Pinning text-xs here
    means the empty message reads identically no matter what font-size
    context the caller's panel happens to be in.
  -->
  <p class="text-xs text-muted-foreground italic">
    <slot />
  </p>
</template>
