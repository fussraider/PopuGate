<template>
  <div>
    <PageHeader>
      <button class="btn btn-ghost btn-sm" @click="handleRefresh">
        <RefreshCw :size="16" />
      </button>
    </PageHeader>

    <DataTable
      :columns="columns"
      :items="schedulerStore.tasks"
      :loading="schedulerStore.loading"
      :empty-icon="CalendarClock"
      :empty-message="t('scheduler.empty')"
      row-key="name"
    >
      <template #cell-name="{ item }">
        <div class="task-name-cell">
          <div class="task-title">{{ t(`scheduler.tasks.${item.name}.name`) }}</div>
          <div class="task-description">{{ t(`scheduler.tasks.${item.name}.description`) }}</div>
          <div v-if="hasWarning(item)" class="task-warning">
            <AlertTriangle :size="12" />
            <span>{{ t(`scheduler.tasks.${item.name}.warning`) }}</span>
          </div>
        </div>
      </template>

      <template #cell-schedule="{ item }">
        <div class="schedule-cell">
          <code>{{ item.effective_schedule }}</code>
          <span v-if="item.default_disabled && !item.enabled" class="badge badge-warning" style="font-size: 10px; padding: 1px 6px;">
            {{ t('scheduler.off_by_default') }}
          </span>
          <span v-if="item.is_overridden" class="badge badge-info" style="font-size: 10px; padding: 1px 6px;">
            {{ t('scheduler.default_label') }}: {{ item.default_schedule }}
          </span>
        </div>
      </template>

      <template #cell-enabled="{ item }">
        <label class="toggle-switch">
          <input
            type="checkbox"
            :checked="item.enabled"
            :disabled="schedulerStore.toggling === item.name"
            @change="handleToggle(item)"
          />
          <span class="toggle-slider" />
        </label>
      </template>

      <template #cell-last_run="{ item }">
        <div v-if="item.last_run" class="last-run-cell">
          <StatusBadge :variant="item.last_run.status === 'success' ? 'success' : 'danger'">
            {{ item.last_run.status === 'success' ? t('scheduler.success') : t('scheduler.error') }}
          </StatusBadge>
          <span class="run-time">{{ formatDate(item.last_run.started_at) }}</span>
          <span class="run-duration">({{ item.last_run.duration_ms }}ms)</span>
          <button
            v-if="item.last_run.status === 'error' && item.last_run.error"
            class="btn btn-ghost btn-sm btn-danger-text"
            @click="showError(item.last_run)"
          >
            <AlertCircle :size="14" />
          </button>
        </div>
        <span v-else class="text-muted">{{ t('scheduler.never_run') }}</span>
      </template>

      <template #mobile-actions="{ item }">
        <button class="btn btn-ghost btn-sm" @click="schedulerActions.open(item)">
          <MoreVertical :size="16" />
        </button>
      </template>
      <template #actions="{ item }">
        <div class="actions-desktop">
          <button
            class="btn btn-ghost btn-sm"
            :title="t('scheduler.run_now')"
            :disabled="schedulerStore.running === item.name"
            @click="handleRunNow(item)"
          >
            <Loader2 v-if="schedulerStore.running === item.name" :size="16" class="animate-spin" />
            <Play v-else :size="16" />
          </button>
          <button class="btn btn-ghost btn-sm" v-tooltip="t('scheduler.edit_schedule')" @click="openEditSchedule(item)">
            <Pencil :size="16" />
          </button>
          <button class="btn btn-ghost btn-sm" v-tooltip="t('scheduler.view_history')" @click="openHistory(item)">
            <History :size="16" />
          </button>
        </div>
      </template>
    </DataTable>

    <!-- Edit Schedule Modal -->
    <FormModal
      v-model="showEditModal"
      :title="t('scheduler.edit_title')"
      :submitting="submitting"
      @submit="handleSaveSchedule"
    >
      <div class="form-group">
        <label class="form-label">{{ t('scheduler.task_name') }}</label>
        <span>{{ editingTask ? taskName(editingTask.name) : '' }}</span>
      </div>
      <div class="form-group">
        <label class="form-label">{{ t('scheduler.default_schedule') }}</label>
        <code>{{ editingTask?.default_schedule }}</code>
      </div>
      <div class="form-group">
        <label class="form-label">{{ t('scheduler.custom_schedule') }}</label>
        <input
          v-model="editSchedule"
          type="text"
          class="form-input"
          :placeholder="t('scheduler.placeholder')"
        />
      </div>
      <div v-if="editSchedule && editSchedule !== editingTask?.default_schedule" class="form-group">
        <button type="button" class="btn btn-warning btn-sm" @click="editSchedule = ''">
          {{ t('scheduler.reset_default') }}
        </button>
      </div>
    </FormModal>

    <!-- History Modal -->
    <Modal v-model="showHistoryModal" :title="historyModalTitle" max-width="700px">
      <div v-if="schedulerStore.history.length === 0" class="text-muted">
        {{ t('scheduler.history_empty') }}
      </div>
      <div v-else class="history-list">
        <div
          v-for="rec in schedulerStore.history"
          :key="rec.id"
          :class="['history-item', { 'history-item-error': rec.status === 'error' }]"
        >
          <div class="history-header">
            <StatusBadge :variant="rec.status === 'success' ? 'success' : 'danger'">
              {{ rec.status === 'success' ? t('scheduler.success') : t('scheduler.error') }}
            </StatusBadge>
            <span class="history-time">{{ formatDate(rec.started_at) }}</span>
            <span class="history-duration">{{ rec.duration_ms }}ms</span>
          </div>
          <div v-if="rec.error" class="history-error">
            <pre>{{ rec.error }}</pre>
          </div>
        </div>
      </div>
    </Modal>

    <!-- Error Detail Modal -->
    <Modal v-model="showErrorModal" :title="t('scheduler.view_error')" max-width="600px">
      <pre class="error-detail">{{ errorDetail }}</pre>
    </Modal>

    <!-- Mobile Action Sheet -->
    <ActionSheet v-model="schedulerActions.isOpen.value" :title="schedulerActions.activeItem.value ? taskName(schedulerActions.activeItem.value.name) : ''">
      <button class="action-sheet-item" :disabled="schedulerStore.running === schedulerActions.activeItem.value?.name"
              @click="handleRunNow(schedulerActions.activeItem.value!); schedulerActions.close()">
        <Play :size="16" /> {{ t('scheduler.run_now') }}
      </button>
      <button class="action-sheet-item" @click="openEditSchedule(schedulerActions.activeItem.value!); schedulerActions.close()">
        <Pencil :size="16" /> {{ t('scheduler.edit_schedule') }}
      </button>
      <button class="action-sheet-item" @click="openHistory(schedulerActions.activeItem.value!); schedulerActions.close()">
        <History :size="16" /> {{ t('scheduler.view_history') }}
      </button>
    </ActionSheet>

    <ConfirmDialog v-bind="confirmState" @confirm="handleConfirm" @cancel="handleCancel" />
  </div>
</template>

<script setup lang="ts">
import {onMounted, ref} from 'vue'
import {useI18n} from 'vue-i18n'
import {useSchedulerStore} from '@/stores/scheduler'
import {useToastStore} from '@/stores/toast'
import {formatDate} from '@/utils/format'
import {useConfirmDialog} from '@/composables/useConfirmDialog'
import {useActionMenu} from '@/composables/useActionMenu'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import FormModal from '@/components/common/FormModal.vue'
import Modal from '@/components/common/Modal.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import ActionSheet from '@/components/common/ActionSheet.vue'
import {
  AlertCircle,
  AlertTriangle,
  CalendarClock,
  History,
  Loader2,
  MoreVertical,
  Pencil,
  Play,
  RefreshCw,
} from '@lucide/vue'
import type {SchedulerHistoryRecord, SchedulerTask} from '@/types/models'

const { t, te } = useI18n()
const schedulerStore = useSchedulerStore()
const toast = useToastStore()
const schedulerActions = useActionMenu()
const { confirmState, confirm, handleConfirm, handleCancel } = useConfirmDialog()

const columns = [
  { key: 'name', header: t('scheduler.table.name') },
  { key: 'schedule', header: t('scheduler.table.schedule') },
  { key: 'enabled', header: t('scheduler.table.enabled'), width: '100px' },
  { key: 'last_run', header: t('scheduler.table.last_run') },
]

const submitting = ref(false)
const showEditModal = ref(false)
const showHistoryModal = ref(false)
const showErrorModal = ref(false)
const editingTask = ref<SchedulerTask | null>(null)
const editSchedule = ref('')
const historyModalTitle = ref('')
const errorDetail = ref('')

function taskName(key: string): string {
  return t(`scheduler.tasks.${key}.name`)
}

function hasWarning(task: SchedulerTask): boolean {
  const key = `scheduler.tasks.${task.name}.warning`
  return te(key) && t(key) !== key
}

function handleRefresh() {
  schedulerStore.load()
}

async function handleToggle(task: SchedulerTask) {
  const newEnabled = !task.enabled
  if (!newEnabled) {
    const ok = await confirm({
      title: t('scheduler.disabled_label'),
      message: t('scheduler.confirm_disable', { name: taskName(task.name) }),
      confirmText: t('scheduler.disabled_label'),
    })
    if (!ok) return
  } else if (hasWarning(task)) {
    const ok = await confirm({
      title: t('scheduler.confirm_enable_warning'),
      message: t(`scheduler.tasks.${task.name}.warning`),
      confirmText: t('scheduler.enabled_label'),
      variant: 'warning',
    })
    if (!ok) return
  }
  try {
    await schedulerStore.toggle(task.name, newEnabled)
    toast.success(newEnabled
      ? t('scheduler.task_enabled', { name: taskName(task.name) })
      : t('scheduler.task_disabled', { name: taskName(task.name) }),
    )
  } catch (e: any) {
    await schedulerStore.load()
  }
}

async function handleRunNow(task: SchedulerTask) {
  const result = await schedulerStore.runNow(task.name)
  if (result) {
    toast.success(t('scheduler.task_triggered'))
    // Reload to get fresh last_run data
    await schedulerStore.load()
  } else {
    toast.error(t('scheduler.task_trigger_failed'))
  }
}

function openEditSchedule(task: SchedulerTask) {
  editingTask.value = task
  editSchedule.value = task.effective_schedule !== task.default_schedule
    ? task.effective_schedule
    : ''
  showEditModal.value = true
}

async function handleSaveSchedule() {
  if (!editingTask.value) return
  submitting.value = true
  try {
    await schedulerStore.updateSchedule(
      editingTask.value.name,
      editSchedule.value || editingTask.value.default_schedule,
    )
    toast.success(t('scheduler.schedule_updated'))
    showEditModal.value = false
  } catch {
    // interceptor handles error toast
  } finally {
    submitting.value = false
  }
}

async function openHistory(task: SchedulerTask) {
  historyModalTitle.value = `${t('scheduler.history_title')}: ${taskName(task.name)}`
  await schedulerStore.loadTaskHistory(task.name, 30)
  showHistoryModal.value = true
}

function showError(record: SchedulerHistoryRecord) {
  errorDetail.value = record.error || t('scheduler.no_errors')
  showErrorModal.value = true
}

onMounted(() => schedulerStore.load())
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.task-name-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.task-title {
  font-weight: 500;
}

.task-description {
  font-size: $font-size-xs;
  color: $text-secondary;
}

.task-warning {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 2px;
  font-size: $font-size-xs;
  color: $color-warning;
}

.schedule-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.last-run-cell {
  display: flex;
  align-items: center;
  gap: $spacing-xs;
  flex-wrap: wrap;
}

.run-time {
  font-size: $font-size-xs;
  color: $text-secondary;
}

.run-duration {
  font-size: $font-size-xs;
  color: $text-secondary;
}

.text-muted {
  color: $text-secondary;
  font-size: $font-size-sm;
}

// Toggle switch styles
.toggle-switch {
  position: relative;
  display: inline-block;
  width: 36px;
  height: 20px;
  cursor: pointer;

  input {
    opacity: 0;
    width: 0;
    height: 0;

    &:disabled + .toggle-slider {
      opacity: 0.5;
      cursor: not-allowed;
    }
  }

  .toggle-slider {
    position: absolute;
    inset: 0;
    background-color: $border-color;
    border-radius: 20px;
    transition: $transition-fast;

    &::before {
      content: '';
      position: absolute;
      height: 14px;
      width: 14px;
      left: 3px;
      bottom: 3px;
      background-color: $color-white;
      border-radius: 50%;
      transition: $transition-fast;
    }
  }

  input:checked + .toggle-slider {
    background-color: $color-primary;

    &::before {
      transform: translateX(16px);
    }
  }
}

// History list
.history-list {
  display: flex;
  flex-direction: column;
  gap: $spacing-sm;
  max-height: 400px;
  overflow-y: auto;
}

.history-item {
  padding: $spacing-sm;
  border: 1px solid $border-color;
  border-radius: $border-radius;
  background: $bg-card;
}

.history-item-error {
  border-color: $color-danger;
}

.history-header {
  display: flex;
  align-items: center;
  gap: $spacing-sm;
}

.history-time {
  font-size: $font-size-xs;
  color: $text-secondary;
}

.history-duration {
  font-size: $font-size-xs;
  color: $text-secondary;
  margin-left: auto;
}

.history-error {
  margin-top: $spacing-xs;

  pre {
    font-size: $font-size-xs;
    color: $color-danger;
    white-space: pre-wrap;
    word-break: break-all;
    margin: 0;
    padding: $spacing-xs;
    background: $color-danger-bg;
    border-radius: $border-radius-sm;
  }
}

.error-detail {
  font-size: $font-size-sm;
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
  padding: $spacing-md;
  background: $color-danger-bg;
  border-radius: $border-radius;
  color: $color-danger;
}
</style>
