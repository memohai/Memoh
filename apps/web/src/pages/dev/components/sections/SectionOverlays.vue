<script setup lang="ts">
// Overlays: trigger-driven floating surfaces. Dialog and the command palette
// use local open refs; the rest open via their own triggers.
import { Plus } from 'lucide-vue-next'
import { ref } from 'vue'
import {
  Button,
  Command, CommandDialog, CommandEmpty, CommandGroup, CommandInput,
  CommandItem, CommandList, CommandSeparator, CommandShortcut,
  ContextMenu, ContextMenuCheckboxItem, ContextMenuContent, ContextMenuItem,
  ContextMenuLabel, ContextMenuSeparator, ContextMenuShortcut, ContextMenuTrigger,
  Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter,
  DialogHeader, DialogTitle, DialogTrigger,
  HoverCard, HoverCardContent, HoverCardTrigger,
  Kbd,
  DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuItem,
  DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuShortcut, DropdownMenuTrigger,
  Popover, PopoverContent, PopoverTrigger,
  Sheet, SheetClose, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle, SheetTrigger,
  Tooltip, TooltipContent, TooltipProvider, TooltipTrigger,
} from '@memohai/ui'
import SectionShell from '../components/SectionShell.vue'
import Specimen from '../components/Specimen.vue'

const dialogOpen = ref(false)
const sheetOpen = ref(false)
const cmdOpen = ref(false)
const showStatusBar = ref(true)
</script>

<template>
  <SectionShell
    id="overlays"
    label="Overlays"
    description="Dialogs, sheets, menus, command palette, tooltips. Click triggers to open."
  >
    <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <Specimen label="<Dialog>">
        <Dialog v-model:open="dialogOpen">
          <DialogTrigger as-child>
            <Button variant="default">
              Open dialog
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Dialog title</DialogTitle>
              <DialogDescription>A short description of what this dialog is for.</DialogDescription>
            </DialogHeader>
            <p class="text-sm text-muted-foreground">
              Body content goes here.
            </p>
            <DialogFooter>
              <DialogClose as-child>
                <Button variant="outline">
                  Cancel
                </Button>
              </DialogClose>
              <Button variant="primary">
                Confirm
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </Specimen>

      <Specimen label="<Sheet>">
        <Sheet v-model:open="sheetOpen">
          <SheetTrigger as-child>
            <Button variant="default">
              Open sheet
            </Button>
          </SheetTrigger>
          <SheetContent side="right">
            <SheetHeader>
              <SheetTitle>Sheet title</SheetTitle>
              <SheetDescription>Slides in from the edge.</SheetDescription>
            </SheetHeader>
            <div class="px-4 text-sm text-muted-foreground">
              Sheet body.
            </div>
            <SheetFooter>
              <SheetClose as-child>
                <Button variant="outline">
                  Close
                </Button>
              </SheetClose>
            </SheetFooter>
          </SheetContent>
        </Sheet>
      </Specimen>

      <Specimen label="<Popover>">
        <Popover>
          <PopoverTrigger as-child>
            <Button variant="default">
              Open popover
            </Button>
          </PopoverTrigger>
          <PopoverContent class="w-64">
            <div class="text-sm">
              <p class="font-medium">
                Popover
              </p>
              <p class="mt-1 text-muted-foreground">
                Floating panel anchored to the trigger.
              </p>
            </div>
          </PopoverContent>
        </Popover>
      </Specimen>

      <Specimen
        label="<HoverCard>"
        note="opens on hover (not click) — same surface as Popover"
      >
        <HoverCard>
          <HoverCardTrigger as-child>
            <Button variant="link">
              @memoh
            </Button>
          </HoverCardTrigger>
          <HoverCardContent>
            <div class="text-sm">
              <p class="font-medium">
                Memoh
              </p>
              <p class="mt-1 text-muted-foreground">
                Multi-member, structured long-memory AI agent platform. Hover previews
                open without a click.
              </p>
            </div>
          </HoverCardContent>
        </HoverCard>
      </Specimen>

      <Specimen label="<DropdownMenu>">
        <DropdownMenu>
          <DropdownMenuTrigger as-child>
            <Button variant="default">
              Open menu
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent class="w-48">
            <DropdownMenuLabel>My account</DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem>
              Profile
              <DropdownMenuShortcut>⇧⌘P</DropdownMenuShortcut>
            </DropdownMenuItem>
            <DropdownMenuItem>Settings</DropdownMenuItem>
            <DropdownMenuCheckboxItem v-model="showStatusBar">
              Status bar
            </DropdownMenuCheckboxItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem class="text-destructive">
              Log out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </Specimen>

      <Specimen label="<ContextMenu>">
        <ContextMenu>
          <ContextMenuTrigger
            class="flex h-20 w-full items-center justify-center rounded-md border border-dashed border-border text-xs text-muted-foreground"
          >
            Right-click here
          </ContextMenuTrigger>
          <ContextMenuContent class="w-48">
            <ContextMenuLabel>Actions</ContextMenuLabel>
            <ContextMenuSeparator />
            <ContextMenuItem>
              Back
              <ContextMenuShortcut>⌘[</ContextMenuShortcut>
            </ContextMenuItem>
            <ContextMenuItem>Reload</ContextMenuItem>
            <ContextMenuCheckboxItem :model-value="true">
              Show bookmarks
            </ContextMenuCheckboxItem>
          </ContextMenuContent>
        </ContextMenu>
      </Specimen>

      <Specimen
        label="<Tooltip>"
        note="terse inverted hint — optional Kbd shortcut chip"
      >
        <TooltipProvider :delay-duration="0">
          <div class="flex items-center gap-3">
            <Tooltip>
              <TooltipTrigger as-child>
                <Button variant="outline">
                  Hover me
                </Button>
              </TooltipTrigger>
              <TooltipContent>Tooltip content</TooltipContent>
            </Tooltip>

            <Tooltip>
              <TooltipTrigger as-child>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label="Add files and more"
                >
                  <Plus />
                </Button>
              </TooltipTrigger>
              <TooltipContent class="flex items-center gap-1.5">
                Add files and more
                <Kbd>/</Kbd>
              </TooltipContent>
            </Tooltip>
          </div>
        </TooltipProvider>
      </Specimen>

      <div class="lg:col-span-2">
        <Specimen label="<Command> inline + <CommandDialog>">
          <div class="flex w-full flex-col gap-3">
            <Command class="max-w-sm border border-[color:var(--border-menu)] shadow-[var(--shadow-dropdown)]">
              <CommandInput placeholder="Type a command or search..." />
              <CommandList>
                <CommandEmpty>No results found.</CommandEmpty>
                <CommandGroup heading="Suggestions">
                  <CommandItem value="calendar">
                    Calendar
                  </CommandItem>
                  <CommandItem value="search">
                    Search
                    <CommandShortcut>⌘S</CommandShortcut>
                  </CommandItem>
                </CommandGroup>
                <CommandSeparator />
                <CommandGroup heading="Settings">
                  <CommandItem value="profile">
                    Profile
                  </CommandItem>
                </CommandGroup>
              </CommandList>
            </Command>

            <div>
              <Button
                variant="default"
                @click="cmdOpen = true"
              >
                Open command dialog
              </Button>
              <CommandDialog v-model:open="cmdOpen">
                <CommandInput placeholder="Type a command..." />
                <CommandList>
                  <CommandEmpty>No results found.</CommandEmpty>
                  <CommandGroup heading="Suggestions">
                    <CommandItem value="new">
                      New file
                    </CommandItem>
                    <CommandItem value="open">
                      Open...
                    </CommandItem>
                  </CommandGroup>
                </CommandList>
              </CommandDialog>
            </div>
          </div>
        </Specimen>
      </div>
    </div>
  </SectionShell>
</template>
