<template>
  <!-- 结构:一句指引 → 英雄码 → 唯一动作 → 倒计时。验证码是给人读的展示物,
       不是输入框;动作只有一颗(复制并打开,同一次手势里完成)。
       —— 间距是本面板单独设计的,不套卡内通用模板(space-y-4 均布) ——
       注意这不是"输入 OTP"屏,是"展示 code 让用户带走"屏(方向相反),且码是
       字母串、容易和句子文本发生视觉牵连,又不能用卡内嵌卡把它框出来 —— 所以
       靠留白隔离,间距最终裁决为均布 gap-4(人眼定稿 2026-07-13):曾试过
       1.5/5 的强分组和 2/4/2.5 的渐变,都读得出"刻意分坨";层次全部交给字号
       (xs↔3xl)和元素顺序表达,留白保持匀速 —— 均布反而最自然。别再调回
       不均匀节奏,除非人眼重新裁决。
       图标位置即语义:外链箭头放尾缀 = 这颗按钮的落点是"打开页面"(复制只是
       顺带);若把 Copy 图标放前缀,按钮读作"复制为主" —— 位置选错会误导预期。
       图标间距由 Button 自带的 gap 负责,不许再手加 ml/mr(叠加会把内容挤偏)。 -->
  <div class="flex flex-col items-center gap-4 text-center">
    <p class="text-xs text-muted-foreground">
      {{ hint }}
    </p>
    <div
      class="font-mono text-3xl font-medium tracking-widest select-all"
      :class="expired ? 'text-muted-foreground line-through' : 'text-foreground'"
    >
      {{ code }}
    </div>
    <Button
      v-if="!expired"
      type="button"
      variant="outline"
      @click="copyAndOpen"
    >
      {{ $t('deviceCode.copyAndOpen') }}
      <ExternalLink />
    </Button>
    <Button
      v-else
      type="button"
      variant="outline"
      :loading="retryLoading"
      loading-mode="manual"
      @click="$emit('retry')"
    >
      <!-- manual loading:同尺寸 spinner 原位替换图标,文字不动(同 Connect 按钮)。 -->
      <Spinner v-if="retryLoading" />
      <RefreshCw v-else />
      {{ $t('deviceCode.retry') }}
    </Button>
    <p
      v-if="expiresAt"
      class="text-xs tabular-nums"
      :class="expired ? 'text-destructive' : 'text-muted-foreground'"
    >
      {{ expired ? $t('deviceCode.codeExpired') : $t('deviceCode.expiresIn', { time: remainingLabel }) }}
    </p>
  </div>
</template>

<script setup lang="ts">
// DeviceCodePanel — 设备码授权(RFC 8628)的"输码时刻"面板。仓库里同一形状已出现
// 三处(providers OAuth、bots ACP、onboarding Step4),此为 owner。
//
// 契约(试点期踩出来的,改动前先读):
// - 动作语义分两层:面板只拥有"复制并打开"与过期后的"重新获取"(emit retry);
//   "取消整个流程"属于 caller 的入口控件(providers 页是行上的 Connect↔Cancel
//   开关) —— 两个动作语义不同,不是重复,不要合并或砍掉其中一个。
// - 复制并打开的顺序有讲究:新标签页必须在用户手势内同步打开(等剪贴板 Promise
//   回来再 open 会被弹窗拦截),所以先开空白页占位,复制失败再收回、只报错不跳转。
// - 倒计时是面板唯一的活性信号(不放 spinner 行;授权轮询是 caller 的事,静默),
//   秒级 ticker 带页面可见性守卫,后台 tab 不空转,回前台立即校准。
// - 防钓鱼提示归属 caller 的 hint 文案(ACP 的 hint 即含警示语);面板不内置,
//   因为"复制并打开"已保证用户只会到达后端下发的官方地址。
// - expiresAt 缺省时不渲染倒计时行,也永不判过期(某些流程不下发过期时间)。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Button, Spinner, toast } from '@felinic/ui'
import { ExternalLink, RefreshCw } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { useClipboard } from '@/composables/useClipboard'

const props = withDefaults(defineProps<{
  /** 一次性用户码,原样展示(上游自带分组连字符)。 */
  code: string
  /** 官方验证页地址,来自后端 —— 面板不做任何拼接。 */
  verificationUri: string
  /** ISO 过期时间;缺省则不显示倒计时、永不判过期。 */
  expiresAt?: string
  /** 指引文案(caller 的 i18n;含防钓鱼警示时也写在这里)。 */
  hint: string
  /** 过期后"重新获取"按钮的 loading 态,与 caller 的签发请求对齐。 */
  retryLoading?: boolean
}>(), {
  expiresAt: '',
  retryLoading: false,
})

const emit = defineEmits<{
  /** 码过期后用户点了"重新获取" —— caller 负责签发新码并更新 props。 */
  retry: []
}>()
void emit

const { t } = useI18n()
const { copyText } = useClipboard()

const nowMs = ref(Date.now())
let ticker: ReturnType<typeof setInterval> | undefined

function stopTicker() {
  if (ticker !== undefined) {
    clearInterval(ticker)
    ticker = undefined
  }
}

// 回前台立即校准一次(后台期间 tick 被跳过,nowMs 是旧的)。
function onVisibilityChange() {
  if (document.visibilityState === 'visible') nowMs.value = Date.now()
}

const expiresAtMs = computed(() => {
  if (!props.expiresAt) return 0
  const ms = new Date(props.expiresAt).getTime()
  return Number.isNaN(ms) ? 0 : ms
})

const expired = computed(() => expiresAtMs.value > 0 && nowMs.value >= expiresAtMs.value)

const remainingLabel = computed(() => {
  const ms = Math.max(0, expiresAtMs.value - nowMs.value)
  const totalSec = Math.ceil(ms / 1000)
  const m = Math.floor(totalSec / 60)
  const s = totalSec % 60
  return `${m}:${String(s).padStart(2, '0')}`
})

watch(expiresAtMs, (expiresAt) => {
  stopTicker()
  if (!expiresAt) return
  nowMs.value = Date.now()
  ticker = setInterval(() => {
    // 页面不可见时跳过:后台 tab 不需要每秒重渲染倒计时。
    if (document.visibilityState === 'hidden') return
    nowMs.value = Date.now()
    if (nowMs.value >= expiresAt) stopTicker()
  }, 1000)
}, { immediate: true })

onMounted(() => {
  document.addEventListener('visibilitychange', onVisibilityChange)
})
onBeforeUnmount(() => {
  stopTicker()
  document.removeEventListener('visibilitychange', onVisibilityChange)
})

async function copyAndOpen() {
  const userCode = props.code.trim()
  const verificationUri = props.verificationUri.trim()
  if (!userCode || !verificationUri) return

  const tab = window.open('', '_blank')
  const copied = await copyText(userCode)
  if (!copied) {
    tab?.close()
    toast.error(t('deviceCode.copyFailed'))
    return
  }
  if (tab) {
    tab.location.href = verificationUri
    tab.focus()
    return
  }
  window.open(verificationUri, '_blank')
}
</script>
