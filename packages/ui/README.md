# @memohai/ui

UI components for Memoh.

## Installation

```bash
npm install @memohai/ui
```

## Usage

Import the CSS file in your `style.css` file.

```css
@import "tailwindcss";
@import "tw-animate-css";
@import "@memohai/ui/style.css";
```

And then you can use the components in your Vue components.

```vue
<script setup>
import { Button } from '@memohai/ui';
</script>

<template>
  <Button>Click me</Button>
</template>
```