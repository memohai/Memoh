<template>
  <!-- Vertical rhythm for a run of FieldStacks. space-y-4 is the field→field gap
       in the house form; keeping it in one wrapper stops each caller re-deriving
       the value and lets the gap change in one place. -->
  <div class="space-y-4">
    <slot />
  </div>
</template>
