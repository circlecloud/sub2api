<template>
  <div class="flex flex-wrap items-center gap-3">
    <div class="relative" ref="bulkActionDropdownRef">
      <button
        @click="toggleBulkActionDropdown"
        :disabled="bulkActionMenuDisabled"
        class="btn btn-secondary"
      >
        <span>{{ bulkActionMenuLabel }}</span>
        <Icon name="chevronDown" size="sm" class="ml-1.5" />
      </button>
      <transition name="dropdown">
        <div
          v-if="showBulkActionDropdown"
          class="dropdown right-0 mt-2 w-48"
        >
          <button
            @click="emitBulkAction('edit-filtered')"
            class="dropdown-item w-full justify-between text-left"
          >
            <span class="flex items-center gap-2">
              <Icon name="edit" size="sm" />
              {{ t('admin.accounts.bulkActions.editAccount') }}
            </span>
          </button>
          <button
            @click="emitBulkAction('test-filtered')"
            class="dropdown-item w-full justify-between text-left"
          >
            <span class="flex items-center gap-2">
              <Icon name="play" size="sm" />
              {{ t('admin.accounts.bulkActions.testAccount') }}
            </span>
          </button>
          <button
            @click="emitBulkAction('refresh-usage-filtered')"
            class="dropdown-item w-full justify-between text-left"
          >
            <span class="flex items-center gap-2">
              <Icon name="refresh" size="sm" />
              {{ t('admin.accounts.bulkActions.refreshUsage') }}
            </span>
          </button>
        </div>
      </transition>
    </div>
    <button @click="$emit('open-import')" class="btn btn-secondary">
      {{ t('admin.accounts.dataImport') }}
    </button>
    <button @click="$emit('open-public-links')" class="btn btn-secondary">
      {{ t('admin.accounts.publicAddLinks.title') }}
    </button>
    <button @click="$emit('open-export')" class="btn btn-secondary">
      {{ selectedCount ? t('admin.accounts.dataExportSelected') : t('admin.accounts.dataExport') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  bulkActionMenuLabel: string
  bulkActionMenuDisabled: boolean
  selectedCount: number
}>()

const emit = defineEmits<{
  (e: 'edit-filtered'): void
  (e: 'test-filtered'): void
  (e: 'refresh-usage-filtered'): void
  (e: 'open-import'): void
  (e: 'open-public-links'): void
  (e: 'open-export'): void
}>()

const { t } = useI18n()

const bulkActionDropdownRef = ref<HTMLElement | null>(null)
const showBulkActionDropdown = ref(false)

const closeBulkActionDropdown = () => {
  showBulkActionDropdown.value = false
}

const toggleBulkActionDropdown = () => {
  if (props.bulkActionMenuDisabled) return
  showBulkActionDropdown.value = !showBulkActionDropdown.value
}

const emitBulkAction = (event: 'edit-filtered' | 'test-filtered' | 'refresh-usage-filtered') => {
  closeBulkActionDropdown()
  if (event === 'edit-filtered') {
    emit('edit-filtered')
    return
  }
  if (event === 'test-filtered') {
    emit('test-filtered')
    return
  }
  emit('refresh-usage-filtered')
}

const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as Node | null
  if (!target) return
  if (bulkActionDropdownRef.value && !bulkActionDropdownRef.value.contains(target)) {
    closeBulkActionDropdown()
  }
}

const handleScroll = () => {
  closeBulkActionDropdown()
}

onMounted(() => {
  window.addEventListener('scroll', handleScroll, true)
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  window.removeEventListener('scroll', handleScroll, true)
  document.removeEventListener('click', handleClickOutside)
})
</script>
