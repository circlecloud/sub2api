<template>
  <GeetestWidget
    v-if="props.provider === 'geetest' && props.geetestCaptchaId"
    ref="widgetRef"
    :captcha-id="props.geetestCaptchaId"
    :mode="props.geetestMode"
    :bind-element="props.geetestBindElement"
    @verify="emit('verify', $event)"
    @expire="emit('expire')"
    @error="emit('error')"
  />
  <TurnstileWidget
    v-else-if="props.provider === 'turnstile' && props.turnstileSiteKey"
    ref="widgetRef"
    :site-key="props.turnstileSiteKey"
    :theme="props.theme"
    :size="props.size"
    @verify="emit('verify', $event)"
    @expire="emit('expire')"
    @error="emit('error')"
  />
</template>

<script setup lang="ts">
import { ref } from 'vue'
import GeetestWidget from './GeetestWidget.vue'
import TurnstileWidget from './TurnstileWidget.vue'

interface CaptchaWidgetHandle {
  reset: () => void
  showCaptcha?: () => boolean
}

const props = withDefaults(
  defineProps<{
    provider: 'turnstile' | 'geetest' | null
    turnstileSiteKey?: string
    geetestCaptchaId?: string
    geetestMode?: 'float' | 'popup' | 'bind'
    geetestBindElement?: string
    theme?: 'light' | 'dark' | 'auto'
    size?: 'normal' | 'compact' | 'flexible'
  }>(),
  {
    turnstileSiteKey: '',
    geetestCaptchaId: '',
    geetestMode: 'float',
    geetestBindElement: '',
    theme: 'auto',
    size: 'flexible'
  }
)

const emit = defineEmits<{
  (e: 'verify', token: string): void
  (e: 'expire'): void
  (e: 'error'): void
}>()

const widgetRef = ref<CaptchaWidgetHandle | null>(null)

const reset = () => {
  widgetRef.value?.reset()
}

const showCaptcha = (): boolean => {
  return widgetRef.value?.showCaptcha?.() ?? false
}

defineExpose({ reset, showCaptcha })
</script>
