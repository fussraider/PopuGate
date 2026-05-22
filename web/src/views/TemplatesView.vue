<template>
  <div>
    <PageHeader>
      <button class="btn btn-primary" @click="addModal.open()">+ {{ t('templates.add') }}</button>
    </PageHeader>

    <DataTable
      :columns="columns"
      :items="templatesStore.templates"
      :loading="templatesStore.loading"
      :empty-icon="LayoutTemplate"
      :empty-message="t('templates.empty')"
      row-key="name"
    >
      <template #cell-max_conns="{ item }">{{ item.max_conns || '∞' }}</template>
      <template #cell-max_ips="{ item }">{{ item.max_ips || '∞' }}</template>
      <template #cell-quota_bytes="{ item }">{{ item.quota_bytes ? formatBytes(item.quota_bytes) : '∞' }}</template>
      <template #cell-expires_days="{ item }">{{ item.expires_days || '∞' }}</template>
      <template #cell-tags="{ item }">
        <span v-for="tag in parseJSONTags(item.tags)" :key="tag" class="badge badge-info tag-badge">{{ tag }}</span>
      </template>
      <template #mobile-actions="{ item }">
        <button class="btn btn-ghost btn-sm" @click="templateActions.open(item)">
          <MoreVertical :size="16" />
        </button>
      </template>
      <template #actions="{ item }">
        <div class="actions-desktop">
          <button class="btn btn-ghost btn-sm" v-tooltip="t('templates.apply')" @click="openApplyModal(item)">
            <Play :size="16" />
          </button>
          <button class="btn btn-ghost btn-sm btn-danger-text" v-tooltip="t('common.delete')" @click="handleRemove(item.name)">
            <Trash2 :size="16" />
          </button>
        </div>
      </template>
    </DataTable>

    <!-- Add Template Modal -->
    <FormModal v-model="addModal.isOpen.value" :title="t('templates.add_title')" :submitting="addModal.submitting.value"
               @submit="handleAdd()">
      <!-- General -->
      <div class="form-card">
        <h4 class="form-card-title">{{ t('templates.section_basic') }}</h4>
        <div class="form-group">
          <label class="form-label">{{ t('templates.name_label') }}</label>
          <input v-model="addModal.form.value.name" class="input" required />
        </div>
      </div>

      <!-- Limits -->
      <div class="form-card">
        <h4 class="form-card-title">{{ t('templates.section_limits') }}</h4>
        <div class="form-row mb-sm">
          <div class="form-group">
            <label class="form-label">{{ t('templates.max_conns') }}</label>
            <input v-model.number="addModal.form.value.max_conns" class="input" type="number" min="0" />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('templates.max_ips') }}</label>
            <input v-model.number="addModal.form.value.max_ips" class="input" type="number" min="0" />
          </div>
        </div>
        <div class="form-row mb-sm">
          <div class="form-group">
            <label class="form-label">{{ t('templates.quota_mb') }}</label>
            <input v-model.number="addModal.form.value.quota_mb" class="input" type="number" min="0" />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('templates.expires_days') }}</label>
            <input v-model.number="addModal.form.value.expires_days" class="input" type="number" min="0" />
          </div>
        </div>
        <div class="form-group mb-sm">
          <label class="form-label">{{ t('templates.tags_label') }}</label>
          <TagInput v-model="addModal.form.value.tags" :placeholder="t('secrets.tags_placeholder')" />
        </div>
        <div class="form-group">
          <label class="form-label">{{ t('templates.notes_label') }}</label>
          <input v-model="addModal.form.value.notes" class="input" />
        </div>
      </div>
    </FormModal>

    <!-- Apply Template Modal -->
    <Modal v-model="applyModal" :title="t('templates.apply_title', { name: applyTemplateName })">
      <p class="mb-sm">{{ t('templates.apply_select') }}</p>
      <div class="form-group mb-md">
        <select v-model="applyTarget" class="input">
          <option value="" disabled>{{ t('templates.apply_placeholder') }}</option>
          <option v-for="s in secretsStore.secrets" :key="s.label" :value="s.label">{{ s.label }}</option>
        </select>
      </div>
      <div class="modal-footer-inline">
        <button class="btn btn-secondary" @click="applyModal = false">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" :disabled="!applyTarget" @click="handleApply">{{ t('templates.apply') }}</button>
      </div>
    </Modal>

    <!-- Mobile Action Sheet -->
    <ActionSheet v-model="templateActions.isOpen.value" :title="templateActions.activeItem.value?.name">
      <button class="action-sheet-item" @click="openApplyModal(templateActions.activeItem.value!); templateActions.close()">
        <Play :size="16" /> {{ t('templates.apply') }}
      </button>
      <button class="action-sheet-item action-danger"
              @click="handleRemove(templateActions.activeItem.value!.name); templateActions.close()">
        <Trash2 :size="16" /> {{ t('common.delete') }}
      </button>
    </ActionSheet>

    <ConfirmDialog v-bind="confirmState" @confirm="handleConfirm" @cancel="handleCancel" />
  </div>
</template>

<script setup lang="ts">
import {computed, onMounted, ref} from 'vue'
import {useI18n} from 'vue-i18n'
import {useSecretsStore, useTemplatesStore} from '@/stores'
import {useToastStore} from '@/stores/toast'
import {formatBytes, parseJSONTags} from '@/utils/format'
import {useConfirmDialog} from '@/composables/useConfirmDialog'
import {useFormModal} from '@/composables/useFormModal'
import {useActionMenu} from '@/composables/useActionMenu'
import Modal from '@/components/common/Modal.vue'
import FormModal from '@/components/common/FormModal.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import TagInput from '@/components/common/TagInput.vue'
import ActionSheet from '@/components/common/ActionSheet.vue'
import {LayoutTemplate, MoreVertical, Play, Trash2} from '@lucide/vue'

const { t } = useI18n()
const templatesStore = useTemplatesStore()
const secretsStore = useSecretsStore()
const toast = useToastStore()
const templateActions = useActionMenu()

const columns = computed(() => [
  { key: 'name', header: t('templates.table.name') },
  { key: 'max_conns', header: t('templates.table.max_conns') },
  { key: 'max_ips', header: t('templates.table.max_ips') },
  { key: 'quota_bytes', header: t('templates.table.quota') },
  { key: 'expires_days', header: t('templates.table.expires') },
  { key: 'tags', header: t('templates.table.tags') },
])

// Add
const addModal = useFormModal({ name: '', max_conns: 0, max_ips: 0, quota_mb: 0, expires_days: 0, tags: '[]', notes: '' })
async function handleAdd() {
  try {
    await addModal.submit(async (f) => {
      await templatesStore.create({
        name: f.name,
        max_conns: f.max_conns,
        max_ips: f.max_ips,
        quota_bytes: f.quota_mb * 1024 * 1024,
        expires_days: f.expires_days,
        tags: f.tags,
        notes: f.notes,
      })
    })
    toast.success(t('templates.created_success', { name: addModal.form.value.name }))
  } catch { /* interceptor handles error toast */ }
}

// Remove
const { confirmState, confirm, handleConfirm, handleCancel } = useConfirmDialog()
async function handleRemove(name: string) {
  if (!await confirm({ title: t('templates.remove_title'), message: t('templates.confirm_remove', { name }), confirmText: t('common.delete') })) return
  try {
    await templatesStore.remove(name)
    toast.success(t('templates.removed_success', { name }))
  } catch { /* interceptor handles error toast */ }
}

// Apply
const applyModal = ref(false)
const applyTemplateName = ref('')
const applyTarget = ref('')

function openApplyModal(item: any) {
  applyTemplateName.value = item.name
  applyTarget.value = ''
  applyModal.value = true
}

async function handleApply() {
  if (!applyTarget.value) return
  try {
    await templatesStore.apply(applyTemplateName.value, applyTarget.value)
    toast.success(t('templates.apply_success', { label: applyTarget.value }))
    applyModal.value = false
  } catch { /* interceptor handles error toast */ }
}

onMounted(() => {
  templatesStore.load()
  secretsStore.load()
})
</script>

<style scoped lang="scss">
.tag-badge {
  font-size: 11px;
  padding: 1px 6px;
}
</style>
