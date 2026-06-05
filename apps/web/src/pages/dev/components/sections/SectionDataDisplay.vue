<script setup lang="ts">
// Data display: composed surfaces for showing content.
import {
  Badge,
  Button,
  ButtonGroup, ButtonGroupSeparator, ButtonGroupText,
  Card, CardAction, CardContent, CardDescription, CardFooter, CardHeader, CardTitle,
  Item, ItemActions, ItemContent, ItemDescription, ItemGroup, ItemMedia, ItemSeparator, ItemTitle,
  ScrollArea, ScrollBar,
  Table, TableBody, TableCaption, TableCell, TableEmpty, TableFooter, TableHead, TableHeader, TableRow,
} from '@memohai/ui'
import { Archive, Folder, Star } from 'lucide-vue-next'
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
      <Specimen label="<Card>">
        <Card class="w-full max-w-sm">
          <CardHeader>
            <CardTitle>Card title</CardTitle>
            <CardDescription>Card description text.</CardDescription>
            <CardAction>
              <Button
                variant="ghost"
                size="icon-sm"
              >
                <Star />
              </Button>
            </CardAction>
          </CardHeader>
          <CardContent class="text-sm text-muted-foreground">
            Main card body content.
          </CardContent>
          <CardFooter class="gap-2">
            <Button
              variant="outline"
              size="sm"
            >
              Cancel
            </Button>
            <Button
              variant="primary"
              size="sm"
            >
              Save
            </Button>
          </CardFooter>
        </Card>
      </Specimen>

      <Specimen label="<Item> / <ItemGroup>">
        <ItemGroup class="w-full max-w-sm">
          <Item variant="default">
            <ItemMedia variant="icon">
              <Folder />
            </ItemMedia>
            <ItemContent>
              <ItemTitle>Documents</ItemTitle>
              <ItemDescription>12 files</ItemDescription>
            </ItemContent>
            <ItemActions>
              <Button
                variant="ghost"
                size="icon-sm"
              >
                <Star />
              </Button>
            </ItemActions>
          </Item>
          <ItemSeparator />
          <Item variant="outline">
            <ItemMedia variant="icon">
              <Archive />
            </ItemMedia>
            <ItemContent>
              <ItemTitle>Archive</ItemTitle>
              <ItemDescription>variant="outline"</ItemDescription>
            </ItemContent>
          </Item>
          <ItemSeparator />
          <Item variant="muted">
            <ItemContent>
              <ItemTitle>Muted item</ItemTitle>
              <ItemDescription>variant="muted"</ItemDescription>
            </ItemContent>
          </Item>
        </ItemGroup>
      </Specimen>

      <div class="lg:col-span-2">
        <Specimen label="<Table> + <TableEmpty>">
          <div class="flex w-full flex-col gap-6">
            <Table>
              <TableCaption>Recent invoices</TableCaption>
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
