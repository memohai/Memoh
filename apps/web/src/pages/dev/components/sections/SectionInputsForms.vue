<script setup lang="ts">
// Inputs & Forms. Local refs back every v-model so the controls are
// interactive on the wall (drag sliders, type, add tags).
import { ref } from 'vue'
import {
  Checkbox,
  Combobox, ComboboxAnchor, ComboboxEmpty, ComboboxGroup, ComboboxInput,
  ComboboxItem, ComboboxItemIndicator, ComboboxList, ComboboxTrigger,
  Field, FieldDescription, FieldError, FieldLabel,
  Form, FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage,
  Input,
  InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput, InputGroupText, InputGroupTextarea,
  InputOTP, InputOTPGroup, InputOTPSeparator, InputOTPSlot,
  Label,
  NativeSelect, NativeSelectOptGroup, NativeSelectOption,
  PinInput, PinInputGroup, PinInputSeparator, PinInputSlot,
  RadioGroup, RadioGroupItem,
  Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectSeparator, SelectTrigger, SelectValue,
  Switch,
  Slider,
  TagsInput, TagsInputInput, TagsInputItem, TagsInputItemDelete, TagsInputItemText,
  Textarea,
  Button,
} from '@memohai/ui'
import { Check, ChevronsUpDown, Eye, EyeOff, Search, X } from 'lucide-vue-next'
import SectionShell from '../components/SectionShell.vue'
import Specimen from '../components/Specimen.vue'

const text = ref('Editable value')
const area = ref('Multiline\ntext')
const checked = ref(true)
const switched = ref(true)
const sliderSingle = ref([40])
const sliderRange = ref([20, 70])
const radioVal = ref('comfortable')
const nativeVal = ref('banana')
const selectVal = ref('')
const tags = ref(['vue', 'tailwind'])
const pin = ref<string[]>([])
const otp = ref('')

const comboItems = ['Apple', 'Banana', 'Cherry', 'Dragonfruit']
const comboVal = ref('')

const clearable = ref('Clear me')
const password = ref('hunter2')
const showPassword = ref(false)

const formSchema = {
  email: (value: string) => (value ? true : 'Email is required'),
}
</script>

<template>
  <SectionShell
    id="inputs-forms"
    label="Inputs & Forms"
    description="Text fields, choosers, and the vee-validate form stack."
  >
    <!-- Input system — edge emphasis (2 states). Same field; focus only.
         'solid' is the everyday default; 'subtle' is for search / low-chrome. -->
    <div class="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-2">
      <Specimen
        label="emphasis solid (default)"
        note="focus turns the edge black"
      >
        <Input
          placeholder="Project name"
          class="w-full"
        />
      </Specimen>
      <Specimen
        label="emphasis subtle"
        note="focus barely deepens the edge"
      >
        <Input
          emphasis="subtle"
          placeholder="Search library"
          class="w-full"
        />
      </Specimen>
    </div>

    <!-- Size scale + composition recipes. These are the usage patterns we WANT
         people to copy, so their styling is tuned here once instead of ad-hoc. -->
    <div class="mb-6 grid grid-cols-1 gap-4 lg:grid-cols-3">
      <Specimen
        label="size scale"
        note="sm · default · lg"
      >
        <div class="flex w-full flex-col gap-2">
          <Input
            size="sm"
            placeholder="Small (sm) — 32px"
          />
          <Input placeholder="Default — 36px" />
          <Input
            size="lg"
            placeholder="Large (lg) — 40px"
          />
        </div>
      </Specimen>

      <Specimen
        label="<Field> wrapper"
        note="label · control · description / error"
      >
        <div class="flex w-full flex-col gap-4">
          <Field>
            <FieldLabel for="f-email">
              Email
            </FieldLabel>
            <Input
              id="f-email"
              type="email"
              placeholder="you@example.com"
            />
            <FieldDescription>We'll never share it.</FieldDescription>
          </Field>
          <Field>
            <FieldLabel for="f-email-bad">
              Email
            </FieldLabel>
            <Input
              id="f-email-bad"
              aria-invalid="true"
              model-value="11"
            />
            <FieldError>Email is not valid.</FieldError>
          </Field>
        </div>
      </Specimen>

      <Specimen
        label="recipes"
        note="clearable · password reveal (InputGroup)"
      >
        <div class="flex w-full flex-col gap-2">
          <InputGroup>
            <InputGroupInput
              v-model="clearable"
              placeholder="Type to clear..."
            />
            <InputGroupAddon
              v-if="clearable"
              align="inline-end"
            >
              <InputGroupButton
                size="icon-xs"
                aria-label="Clear"
                @click="clearable = ''"
              >
                <X />
              </InputGroupButton>
            </InputGroupAddon>
          </InputGroup>
          <InputGroup>
            <InputGroupInput
              v-model="password"
              :type="showPassword ? 'text' : 'password'"
              placeholder="Password"
            />
            <InputGroupAddon align="inline-end">
              <InputGroupButton
                size="icon-xs"
                :aria-label="showPassword ? 'Hide password' : 'Show password'"
                @click="showPassword = !showPassword"
              >
                <EyeOff v-if="showPassword" />
                <Eye v-else />
              </InputGroupButton>
            </InputGroupAddon>
          </InputGroup>
        </div>
      </Specimen>
    </div>

    <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <Specimen label="<Input> states">
        <div class="flex w-full max-w-xs flex-col gap-2">
          <Input v-model="text" />
          <Input placeholder="Placeholder" />
          <Input
            disabled
            placeholder="Disabled"
          />
          <div class="flex flex-col gap-1.5">
            <Input
              aria-invalid="true"
              model-value="11"
            />
            <FieldError>Email is not valid.</FieldError>
          </div>
        </div>
      </Specimen>

      <Specimen label="<Textarea>">
        <Textarea
          v-model="area"
          class="w-full"
        />
      </Specimen>

      <Specimen label="<Checkbox>">
        <Label class="flex items-center gap-2 text-[13px]">
          <Checkbox v-model="checked" /> Checked
        </Label>
        <Label class="flex items-center gap-2 text-[13px]">
          <Checkbox :model-value="false" /> Unchecked
        </Label>
        <Label class="flex items-center gap-2 text-[13px] opacity-60">
          <Checkbox disabled /> Disabled
        </Label>
      </Specimen>

      <Specimen label="<Switch>">
        <Switch v-model="switched" />
        <Switch :model-value="false" />
        <Switch disabled />
      </Specimen>

      <Specimen label="<Slider> single / range / disabled">
        <div class="flex w-full flex-col gap-4">
          <Slider
            v-model="sliderSingle"
            :min="0"
            :max="100"
          />
          <Slider
            v-model="sliderRange"
            :min="0"
            :max="100"
          />
          <Slider
            :model-value="[50]"
            disabled
          />
        </div>
      </Specimen>

      <Specimen label="<RadioGroup>">
        <RadioGroup
          v-model="radioVal"
          class="gap-2"
        >
          <Label class="flex items-center gap-2">
            <RadioGroupItem value="comfortable" /> Comfortable
          </Label>
          <Label class="flex items-center gap-2">
            <RadioGroupItem value="compact" /> Compact
          </Label>
          <Label class="flex items-center gap-2">
            <RadioGroupItem value="spacious" /> Spacious
          </Label>
        </RadioGroup>
      </Specimen>

      <Specimen label="<NativeSelect>">
        <NativeSelect
          v-model="nativeVal"
          class="w-48"
        >
          <NativeSelectOptGroup label="Fruits">
            <NativeSelectOption value="apple">
              Apple
            </NativeSelectOption>
            <NativeSelectOption value="banana">
              Banana
            </NativeSelectOption>
          </NativeSelectOptGroup>
          <NativeSelectOption value="carrot">
            Carrot
          </NativeSelectOption>
        </NativeSelect>
      </Specimen>

      <Specimen label="<Select>">
        <Select v-model="selectVal">
          <SelectTrigger class="w-48">
            <SelectValue placeholder="Pick a fruit" />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectLabel>Fruits</SelectLabel>
              <SelectItem value="apple">
                Apple
              </SelectItem>
              <SelectItem value="banana">
                Banana
              </SelectItem>
              <SelectSeparator />
              <SelectItem value="cherry">
                Cherry
              </SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      </Specimen>

      <Specimen
        label="<Combobox>"
        note="reka-ui contract — first to eyeball-check"
      >
        <Combobox
          v-model="comboVal"
          class="w-56"
        >
          <ComboboxAnchor class="w-56">
            <div class="relative w-full items-center">
              <ComboboxInput
                class="pl-9"
                :display-value="(v) => (v as string) ?? ''"
                placeholder="Search fruit..."
              />
              <span class="absolute inset-y-0 start-0 flex items-center justify-center px-3">
                <Search class="size-4 text-muted-foreground" />
              </span>
              <ComboboxTrigger class="absolute inset-y-0 end-0 flex items-center px-2">
                <ChevronsUpDown class="size-4 text-muted-foreground" />
              </ComboboxTrigger>
            </div>
          </ComboboxAnchor>
          <ComboboxList>
            <ComboboxEmpty>No fruit found.</ComboboxEmpty>
            <ComboboxGroup>
              <ComboboxItem
                v-for="item in comboItems"
                :key="item"
                :value="item"
              >
                {{ item }}
                <ComboboxItemIndicator>
                  <Check class="size-4" />
                </ComboboxItemIndicator>
              </ComboboxItem>
            </ComboboxGroup>
          </ComboboxList>
        </Combobox>
      </Specimen>

      <Specimen label="<TagsInput>">
        <TagsInput
          v-model="tags"
          class="w-64"
        >
          <TagsInputItem
            v-for="item in tags"
            :key="item"
            :value="item"
          >
            <TagsInputItemText />
            <TagsInputItemDelete />
          </TagsInputItem>
          <TagsInputInput placeholder="Add tag..." />
        </TagsInput>
      </Specimen>

      <Specimen label="<PinInput>">
        <PinInput
          v-model="pin"
          placeholder="○"
        >
          <PinInputGroup>
            <PinInputSlot
              v-for="i in 2"
              :key="i"
              :index="i - 1"
            />
            <PinInputSeparator />
            <PinInputSlot
              v-for="i in 2"
              :key="i + 2"
              :index="i + 1"
            />
          </PinInputGroup>
        </PinInput>
      </Specimen>

      <Specimen label="<InputOTP :maxlength>">
        <InputOTP
          v-model="otp"
          :maxlength="6"
        >
          <template #default="{ slots }">
            <InputOTPGroup>
              <InputOTPSlot
                v-for="(slot, i) in slots.slice(0, 3)"
                :key="i"
                :index="i"
              />
            </InputOTPGroup>
            <InputOTPSeparator />
            <InputOTPGroup>
              <InputOTPSlot
                v-for="(slot, i) in slots.slice(3, 6)"
                :key="i + 3"
                :index="i + 3"
              />
            </InputOTPGroup>
          </template>
        </InputOTP>
      </Specimen>

      <Specimen label="<InputGroup>">
        <div class="flex w-full flex-col gap-2">
          <InputGroup>
            <InputGroupAddon>
              <Search class="size-4" />
            </InputGroupAddon>
            <InputGroupInput placeholder="Search..." />
            <InputGroupButton>Go</InputGroupButton>
          </InputGroup>
          <InputGroup>
            <InputGroupInput placeholder="https://" />
            <InputGroupAddon align="inline-end">
              <InputGroupText>.com</InputGroupText>
            </InputGroupAddon>
          </InputGroup>
          <InputGroup>
            <InputGroupTextarea placeholder="Multiline group..." />
          </InputGroup>
        </div>
      </Specimen>

      <div class="lg:col-span-2">
        <Specimen
          label="<Form> + <FormField> (vee-validate)"
          note="submit empty to see FormMessage"
        >
          <Form
            class="w-full max-w-sm space-y-3"
            :validation-schema="formSchema"
            :initial-values="{ email: '' }"
            @submit="() => {}"
          >
            <FormField
              v-slot="{ componentField }"
              name="email"
            >
              <FormItem>
                <FormLabel>Email</FormLabel>
                <FormControl>
                  <Input
                    type="email"
                    placeholder="you@example.com"
                    v-bind="componentField"
                  />
                </FormControl>
                <FormDescription>We'll never share it.</FormDescription>
                <FormMessage />
              </FormItem>
            </FormField>
            <Button
              type="submit"
              size="sm"
            >
              Submit
            </Button>
          </Form>
        </Specimen>
      </div>
    </div>
  </SectionShell>
</template>
