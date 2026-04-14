<template>
  <div v-if="props.captchaId && props.mode !== 'bind'" class="geetest-wrapper">
    <div ref="containerRef" class="geetest-container"></div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'

interface GeeTestInitConfig {
  captchaId: string
  product?: 'float' | 'popup' | 'bind'
  bindElement?: string
  language?: string
  protocol?: 'http://' | 'https://'
  timeout?: number
  nativeButton?: {
    width?: string
    height?: string
  }
}

interface GeeTestValidateResult {
  lot_number: string
  captcha_output: string
  pass_token: string
  gen_time: string
}

interface GeeTestInstance {
  appendTo?: (container: HTMLElement | string) => void
  reset: () => void
  showCaptcha?: () => void
  destroy?: () => void
  getValidate: () => GeeTestValidateResult | false
  onReady: (callback: () => void) => GeeTestInstance
  onSuccess: (callback: (payload?: unknown) => void) => GeeTestInstance
  onError: (callback: (error?: unknown) => void) => GeeTestInstance
  onFail?: (callback: (payload?: unknown) => void) => GeeTestInstance
  onClose?: (callback: () => void) => GeeTestInstance
}

declare global {
  interface Window {
    initGeetest4?: (config: GeeTestInitConfig, callback: (captchaObj: GeeTestInstance) => void) => void
  }
}

const GEETEST_SCRIPT_URL = 'https://static.geetest.com/v4/gt4.js'

const props = withDefaults(
  defineProps<{
    captchaId: string
    mode?: 'float' | 'popup' | 'bind'
    bindElement?: string
  }>(),
  {
    mode: 'float',
    bindElement: ''
  }
)

const emit = defineEmits<{
  (e: 'verify', token: string): void
  (e: 'expire'): void
  (e: 'error'): void
}>()

const containerRef = ref<HTMLElement | null>(null)
const captchaRef = ref<GeeTestInstance | null>(null)
const scriptLoaded = ref(false)
const ready = ref(false)
const verificationCompleted = ref(false)

const loadScript = (): Promise<void> => {
  return new Promise((resolve, reject) => {
    if (window.initGeetest4) {
      scriptLoaded.value = true
      resolve()
      return
    }

    const existingScript = document.querySelector(`script[src="${GEETEST_SCRIPT_URL}"]`) as
      | HTMLScriptElement
      | null
    if (existingScript) {
      if (existingScript.dataset.loaded === 'true') {
        scriptLoaded.value = true
        resolve()
        return
      }

      const handleLoad = () => {
        existingScript.dataset.loaded = 'true'
        scriptLoaded.value = true
        resolve()
      }
      const handleError = () => reject(new Error('Failed to load GeeTest script'))
      existingScript.addEventListener('load', handleLoad, { once: true })
      existingScript.addEventListener('error', handleError, { once: true })
      return
    }

    const script = document.createElement('script')
    script.src = GEETEST_SCRIPT_URL
    script.async = true
    script.defer = true
    script.onload = () => {
      script.dataset.loaded = 'true'
      scriptLoaded.value = true
      resolve()
    }
    script.onerror = () => {
      reject(new Error('Failed to load GeeTest script'))
    }
    document.head.appendChild(script)
  })
}

const resetState = () => {
  ready.value = false
  verificationCompleted.value = false
}

const normalizeValidateResult = (payload: unknown): GeeTestValidateResult | null => {
  if (!payload) {
    return null
  }

  if (typeof payload === 'string') {
    try {
      return normalizeValidateResult(JSON.parse(payload))
    } catch {
      return null
    }
  }

  if (typeof payload !== 'object') {
    return null
  }

  const data = payload as Record<string, unknown>
  const lotNumber = typeof data.lot_number === 'string' ? data.lot_number : data.lotNumber
  const captchaOutput =
    typeof data.captcha_output === 'string' ? data.captcha_output : data.captchaOutput
  const passToken = typeof data.pass_token === 'string' ? data.pass_token : data.passToken
  const genTime = typeof data.gen_time === 'string' ? data.gen_time : data.genTime

  if (
    typeof lotNumber !== 'string' ||
    typeof captchaOutput !== 'string' ||
    typeof passToken !== 'string' ||
    typeof genTime !== 'string'
  ) {
    return null
  }

  return {
    lot_number: lotNumber,
    captcha_output: captchaOutput,
    pass_token: passToken,
    gen_time: genTime
  }
}

const destroyWidget = () => {
  resetState()
  if (captchaRef.value?.destroy) {
    try {
      captchaRef.value.destroy()
    } catch {
      // Ignore destroy errors
    }
  }
  captchaRef.value = null
  if (containerRef.value) {
    containerRef.value.innerHTML = ''
  }
}

const initWidget = () => {
  if (!window.initGeetest4 || !props.captchaId) {
    return
  }

  if (props.mode !== 'bind' && !containerRef.value) {
    return
  }

  if (props.mode === 'bind') {
    const bindSelector = props.bindElement?.trim()
    if (!bindSelector || !document.querySelector(bindSelector)) {
      return
    }
  }

  destroyWidget()

  const config: GeeTestInitConfig = {
    captchaId: props.captchaId,
    product: props.mode,
    protocol: 'https://'
  }

  if (props.mode === 'bind') {
    config.bindElement = props.bindElement.trim()
  } else {
    config.nativeButton = {
      width: '100%',
      height: '44px'
    }
  }

  window.initGeetest4(config, (captchaObj) => {
    captchaRef.value = captchaObj

    captchaObj.onReady(() => {
      ready.value = true
    })

    captchaObj.onSuccess((payload) => {
      const result = normalizeValidateResult(payload) ?? normalizeValidateResult(captchaObj.getValidate())
      if (!result) {
        verificationCompleted.value = false
        emit('error')
        return
      }
      verificationCompleted.value = true
      emit('verify', JSON.stringify(result))
    })

    captchaObj.onError(() => {
      if (verificationCompleted.value) {
        return
      }
      emit('error')
    })

    captchaObj.onFail?.(() => {
      if (verificationCompleted.value) {
        return
      }
      emit('error')
    })

    captchaObj.onClose?.(() => {
      if (verificationCompleted.value) {
        verificationCompleted.value = false
        return
      }
      emit('expire')
    })

    if (props.mode !== 'bind') {
      captchaObj.appendTo?.(containerRef.value!)
    }
  })
}

const reset = () => {
  verificationCompleted.value = false
  captchaRef.value?.reset()
}

const showCaptcha = (): boolean => {
  if (props.mode !== 'bind' || !ready.value || !captchaRef.value?.showCaptcha) {
    return false
  }

  verificationCompleted.value = false

  try {
    captchaRef.value.showCaptcha()
    return true
  } catch {
    emit('error')
    return false
  }
}

defineExpose({ reset, showCaptcha })

onMounted(async () => {
  if (!props.captchaId) {
    return
  }

  try {
    await loadScript()
    initWidget()
  } catch (error) {
    console.error('Failed to initialize GeeTest:', error)
    emit('error')
  }
})

onUnmounted(() => {
  destroyWidget()
})

watch(
  () => [props.captchaId, props.mode, props.bindElement] as const,
  ([newCaptchaId]) => {
    if (newCaptchaId && scriptLoaded.value) {
      initWidget()
    } else if (!newCaptchaId) {
      destroyWidget()
    }
  }
)
</script>

<style scoped>
.geetest-wrapper {
  width: 100%;
}

.geetest-container {
  width: 100%;
  min-height: 44px;
}

.geetest-container :deep(*) {
  box-sizing: border-box;
}

.geetest-container :deep([class*='geetest_holder']),
.geetest-container :deep([class*='geetest_btn_wrap']),
.geetest-container :deep([class*='geetest_btn']) {
  width: 100% !important;
  min-width: 100% !important;
}

.geetest-container :deep([class*='geetest_btn']) {
  height: 44px !important;
  min-height: 44px !important;
  border-radius: 0.75rem !important;
  overflow: hidden !important;
}
</style>
