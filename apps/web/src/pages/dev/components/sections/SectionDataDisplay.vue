<script setup lang="ts">
// Data display: composed surfaces for showing content.
import {
  Badge,
  Button,
  ButtonGroup, ButtonGroupSeparator, ButtonGroupText,
  Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle,
  Item, ItemActions, ItemContent, ItemDescription, ItemGroup, ItemSeparator, ItemTitle,
  ScrollArea, ScrollBar,
  Table, TableBody, TableCell, TableEmpty, TableFooter, TableHead, TableHeader, TableRow,
} from '@memohai/ui'
import SectionShell from '../components/SectionShell.vue'
import Specimen from '../components/Specimen.vue'
import VariantMatrix from '../components/VariantMatrix.vue'
import { variantSpecs } from '../lib/variant-specs'

const invoices = [
  { id: 'INV-001', status: 'Paid', total: '$250.00' },
  { id: 'INV-002', status: 'Pending', total: '$150.00' },
  { id: 'INV-003', status: 'Unpaid', total: '$350.00' },
]
</script>

<template>
  <SectionShell
    id="data-display"
    label="Data Display"
    description="Cards, items, tables, and grouped controls."
  >
    <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <Specimen
        label="<Card>"
        note="an inline surface that lives in the page (settings sections, dashboards, detail panes) — not a popup. it shares Dialog's surface: same bg-card, same rounded-xl, same header/content/footer anatomy. a Dialog is essentially this Card lifted onto a backdrop. use Card for persistent content, Dialog for transient decisions."
      >
        <Card class="w-full max-w-sm gap-3.5">
          <CardHeader class="gap-1">
            <CardTitle>Workspace name</CardTitle>
            <CardDescription>Used across the dashboard and CLI</CardDescription>
          </CardHeader>
          <CardContent class="text-control text-muted-foreground">
            Pick a short, recognizable name. You can change it at any time.
          </CardContent>
          <CardFooter class="justify-end gap-2">
            <Button variant="outline">
              Cancel
            </Button>
            <Button variant="primary">
              Save
            </Button>
          </CardFooter>
        </Card>
      </Specimen>

      <Specimen
        label="<Item> outline — standalone row"
        note="outline is the only variant: a self-contained hairline-bordered row whose fill inherits the surface. a bare <Item> (no variant) is transparent and belongs inside an <ItemGroup>."
      >
        <Item
          variant="outline"
          class="w-full max-w-sm"
        >
          <ItemContent>
            <ItemTitle>Workspace runtime</ItemTitle>
            <ItemDescription>Apple Virtualization · 2 vCPU</ItemDescription>
          </ItemContent>
          <ItemActions>
            <Button
              variant="outline"
              size="sm"
            >
              Configure
            </Button>
          </ItemActions>
        </Item>
      </Specimen>

      <Specimen
        label="<ItemGroup> — action list"
        note="the real pattern: each row is a function — title + description on the left, the action (button) on the right. hairline separators inside one bordered surface, no leading icons"
      >
        <ItemGroup class="w-full max-w-sm rounded-lg border">
          <Item>
            <ItemContent>
              <ItemTitle>Pro plan</ItemTitle>
              <ItemDescription>Unlimited workspaces & priority models</ItemDescription>
            </ItemContent>
            <ItemActions>
              <Button
                variant="primary"
                size="sm"
              >
                Upgrade
              </Button>
            </ItemActions>
          </Item>
          <ItemSeparator />
          <Item>
            <ItemContent>
              <ItemTitle>Team members</ItemTitle>
              <ItemDescription>3 of 5 seats used</ItemDescription>
            </ItemContent>
            <ItemActions>
              <Button
                variant="outline"
                size="sm"
              >
                Invite
              </Button>
            </ItemActions>
          </Item>
          <ItemSeparator />
          <Item>
            <ItemContent>
              <ItemTitle>API keys</ItemTitle>
              <ItemDescription>2 active</ItemDescription>
            </ItemContent>
            <ItemActions>
              <Button
                variant="outline"
                size="sm"
              >
                Manage
              </Button>
            </ItemActions>
          </Item>
        </ItemGroup>
      </Specimen>

      <Specimen
        label="<Item> single line — message + action"
        note="no description: a one-line notice (text left, action right). kept to a normal row width so the standard sm button stays in proportion — no hand-tuned one-off button"
      >
        <Item
          variant="outline"
          class="w-full"
        >
          <ItemContent>
            <ItemTitle>The response was interrupted</ItemTitle>
          </ItemContent>
          <ItemActions>
            <Button
              variant="outline"
              size="sm"
            >
              Retry
            </Button>
          </ItemActions>
        </Item>
      </Specimen>

      <div class="lg:col-span-2">
        <Specimen
          label="<Table> + <TableEmpty>"
          note="a quiet report table: transparent surface + hairline rows + 13px body. rows are inert by default — no decorative hover. a row only earns an interaction color when it is genuinely clickable (<TableRow interactive>), matching the <Item> contract. the title lives outside the table, not as a height-padding <caption>."
        >
          <div class="flex w-full max-w-2xl flex-col gap-6">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Invoice</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead class="text-right">
                    Total
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                <TableRow
                  v-for="inv in invoices"
                  :key="inv.id"
                >
                  <TableCell class="font-medium">
                    {{ inv.id }}
                  </TableCell>
                  <TableCell>
                    <Badge variant="secondary">
                      {{ inv.status }}
                    </Badge>
                  </TableCell>
                  <TableCell class="text-right">
                    {{ inv.total }}
                  </TableCell>
                </TableRow>
              </TableBody>
              <TableFooter>
                <TableRow>
                  <TableCell :colspan="2">
                    Total
                  </TableCell>
                  <TableCell class="text-right">
                    $750.00
                  </TableCell>
                </TableRow>
              </TableFooter>
            </Table>

            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Invoice</TableHead>
                  <TableHead>Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                <TableEmpty :colspan="2">
                  No invoices yet.
                </TableEmpty>
              </TableBody>
            </Table>
          </div>
        </Specimen>
      </div>

      <Specimen label="<ButtonGroup :orientation :size>">
        <VariantMatrix
          :variants="variantSpecs.buttonGroup.variants"
          :sizes="variantSpecs.buttonGroup.sizes"
          axis-label="orientation"
        >
          <template #default="{ variant, size }">
            <ButtonGroup
              :orientation="variant"
              :size="size"
            >
              <Button variant="outline">
                One
              </Button>
              <Button variant="outline">
                Two
              </Button>
              <Button variant="outline">
                Three
              </Button>
            </ButtonGroup>
          </template>
        </VariantMatrix>
      </Specimen>

      <Specimen label="<ButtonGroup> separator + text">
        <ButtonGroup>
          <Button variant="outline">
            Copy
          </Button>
          <ButtonGroupSeparator />
          <ButtonGroupText>or</ButtonGroupText>
          <Button variant="outline">
            Paste
          </Button>
        </ButtonGroup>
      </Specimen>

      <div class="lg:col-span-2">
        <Specimen label="<ScrollArea> vertical + horizontal">
          <div class="flex w-full flex-wrap gap-6">
            <ScrollArea class="h-40 w-56 rounded-md border border-border p-3">
              <p
                v-for="i in 20"
                :key="i"
                class="py-1 text-xs text-muted-foreground"
              >
                Scrollable row {{ i }}
              </p>
            </ScrollArea>
            <ScrollArea class="w-72 whitespace-nowrap rounded-md border border-border p-3">
              <div class="flex gap-3">
                <div
                  v-for="i in 12"
                  :key="i"
                  class="flex size-20 shrink-0 items-center justify-center rounded-md bg-muted text-xs text-muted-foreground"
                >
                  {{ i }}
                </div>
              </div>
              <ScrollBar orientation="horizontal" />
            </ScrollArea>
          </div>
        </Specimen>
      </div>
    </div>
  </SectionShell>
</template>
