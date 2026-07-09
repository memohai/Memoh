<script setup lang="ts">
// Password input with a trailing show/hide toggle. Thin wrapper over the
// InputGroup atoms from @felinic/ui so it inherits the field-edge design
// language and stays consistent with the secret-field pattern in channel-field.
//
// `class` is routed to the InputGroup wrapper (callers like the login page pass
// `bg-background` so the field stays opaque over the animated dot matrix);
// every other attr (id / placeholder / autocomplete / aria-invalid / v-model)
// falls through to the input via $attrs.
import type { HTMLAttributes } from 'vue'
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { InputGroup, InputGroupInput, InputGroupAddon, InputGroupButton } from '@felinic/ui'
import { Eye, EyeOff } from 'lucide-vue-next'

defineOptions({ inheritAttrs: false })

const props = defineProps<{
  class?: HTMLAttributes['class']
  disabled?: boolean
}>()

const { t } = useI18n()
const revealed = ref(false)
</script>

<template>
  <InputGroup :class="props.class">
    <InputGroupInput
      v-bind="$attrs"
      :type="revealed ? 'text' : 'password'"
      :disabled="props.disabled"
    />
    <InputGroupAddon align="inline-end">
      <InputGroupButton
        size="icon-xs"
        variant="quiet"
        type="button"
        :tabindex="props.disabled ? -1 : undefined"
        :disabled="props.disabled"
        :aria-label="revealed ? t('common.hidePassword') : t('common.showPassword')"
        @click="revealed = !revealed"
      >
        <component :is="revealed ? EyeOff : Eye" />
      </InputGroupButton>
    </InputGroupAddon>
  </InputGroup>
</template>
