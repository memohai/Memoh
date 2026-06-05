<script setup lang="ts">
// Layout / structure.
import { ref } from 'vue'
import {
  Button,
  Collapsible, CollapsibleContent, CollapsibleTrigger,
} from '@memohai/ui'
import { ChevronsUpDown } from 'lucide-vue-next'
import SectionShell from '../components/SectionShell.vue'
import Specimen from '../components/Specimen.vue'

const open = ref(false)
</script>

<template>
  <SectionShell
    id="layout"
    label="Layout"
    description="Structural primitives."
  >
    <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <Specimen label="<Collapsible>">
        <Collapsible
          v-model:open="open"
          class="w-full max-w-sm space-y-2"
        >
          <div class="flex items-center justify-between gap-2 rounded-md border border-border px-3 py-2">
            <span class="text-sm font-medium">Toggle details</span>
            <CollapsibleTrigger as-child>
              <Button
                variant="ghost"
                size="icon-sm"
              >
                <ChevronsUpDown />
              </Button>
            </CollapsibleTrigger>
          </div>
          <CollapsibleContent class="space-y-2">
            <div class="rounded-md border border-border px-3 py-2 text-xs text-muted-foreground">
              Hidden content row 1
            </div>
            <div class="rounded-md border border-border px-3 py-2 text-xs text-muted-foreground">
              Hidden content row 2
            </div>
          </CollapsibleContent>
        </Collapsible>
      </Specimen>
    </div>
  </SectionShell>
</template>
