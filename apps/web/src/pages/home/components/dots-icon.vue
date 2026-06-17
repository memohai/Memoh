<!-- Horizontal "more" dots. lucide's `Ellipsis` draws each dot as a *stroked*
     r=1 circle; at icon sizes the stroke nearly fills the circle, leaving a tiny
     hole that reads as a hollow donut ("ring inside a ring"). This draws the
     three dots as solid filled circles instead, so they read as clean dots at
     any size. Same 24px grid as lucide so it inherits size-* and alignment. -->
<template>
  <svg
    xmlns="http://www.w3.org/2000/svg"
    viewBox="0 0 24 24"
    fill="currentColor"
    stroke="none"
    aria-hidden="true"
  >
    <circle
      cx="5"
      cy="12"
      r="1.9"
    />
    <circle
      cx="12"
      cy="12"
      r="1.9"
    />
    <circle
      cx="19"
      cy="12"
      r="1.9"
    />
  </svg>
</template>
