<script setup lang="ts">
// Layout / structure.
import { ref } from 'vue'
import {
  Accordion, AccordionContent, AccordionItem, AccordionTrigger,
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

      <Specimen
        label="<Accordion type=single collapsible>"
        note="height tween · chevron flips · hairline row dividers"
      >
        <Accordion
          type="single"
          collapsible
          default-value="item-1"
          class="max-w-sm"
        >
          <AccordionItem value="item-1">
            <AccordionTrigger>What is a workspace runtime?</AccordionTrigger>
            <AccordionContent>
              An isolated container (or trusted local sandbox) where a bot can edit files,
              run commands, and host tools.
            </AccordionContent>
          </AccordionItem>
          <AccordionItem value="item-2">
            <AccordionTrigger>Which channels are supported?</AccordionTrigger>
            <AccordionContent>
              Telegram, Discord, Lark, DingTalk, WeChat, Matrix, Email, and more.
            </AccordionContent>
          </AccordionItem>
          <AccordionItem value="item-3">
            <AccordionTrigger>Can I run it locally?</AccordionTrigger>
            <AccordionContent>
              Yes — the desktop app runs a local server with SQLite, local workspaces, and a bundled CLI.
            </AccordionContent>
          </AccordionItem>
        </Accordion>
      </Specimen>
    </div>
  </SectionShell>
</template>
