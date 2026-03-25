<template>
  <section class="relative rounded-full p-0.5 border flex-none">
    <img
      src="/logo.png"
      alt="logo.png"
      class="w-6.5"
    >
    <img
      v-if="isLoading"
      src="data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSIyNCIgaGVpZ2h0PSIyNCIgdmlld0JveD0iMCAwIDI0IDI0Ij48cGF0aCBmaWxsPSIjOEI1Q0Y2IiBkPSJNMTIsMUExMSwxMSwwLDEsMCwyMywxMiwxMSwxMSwwLDAsMCwxMiwxWm0wLDE5YTgsOCwwLDEsMSw4LThBOCw4LDAsMCwxLDEyLDIwWiIgb3BhY2l0eT0iMC4yNSIvPjxwYXRoIGZpbGw9IiM4QjVDRjYiIGQ9Ik0xMC4xNCwxLjE2YTExLDExLDAsMCwwLTksOC45MkExLjU5LDEuNTksMCwwLDAsMi40NiwxMiwxLjUyLDEuNTIsMCwwLDAsNC4xMSwxMC43YTgsOCwwLDAsMSw2LjY2LTYuNjFBMS40MiwxLjQyLDAsMCwwLDEyLDIuNjloMEExLjU3LDEuNTcsMCwwLDAsMTAuMTQsMS4xNloiPjxhbmltYXRlVHJhbnNmb3JtIGF0dHJpYnV0ZU5hbWU9InRyYW5zZm9ybSIgZHVyPSIwLjc1cyIgcmVwZWF0Q291bnQ9ImluZGVmaW5pdGUiIHR5cGU9InJvdGF0ZSIgdmFsdWVzPSIwIDEyIDEyOzM2MCAxMiAxMiIvPjwvcGF0aD48L3N2Zz4="
      alt="loading"
      width="13"
      class="absolute bottom-0 right-0 bg-card p-0.5 rounded-full"
    >
    <FontAwesomeIcon
      v-if="error" 
      :icon="['fas', 'circle-exclamation']"
      class="absolute bottom-0 right-0 bg-card p-0.5 w-2.5! rounded-full"
    />
  </section>
</template>

<script setup lang="ts">

withDefaults(defineProps < { isLoading?: boolean ,error?:boolean} > (), {
  isLoading: true,
  error:false
})

</script>