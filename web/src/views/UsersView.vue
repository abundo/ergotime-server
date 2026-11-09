<template>
  <div>
    <div class="flex align-items-center justify-content-between mb-3 flex-wrap gap-2">
      <h1 class="page-title m-0">Users</h1>
      <Button label="New user" icon="pi pi-plus" @click="openCreate" />
    </div>

    <div class="card-panel">
      <DataTable
        :value="items"
        :loading="loading"
        paginator
        :rows="15"
        striped-rows
        size="small"
        data-key="id"
        empty-message="No users"
      >
        <Column field="id" header="ID" style="width: 5rem" />
        <Column field="name" header="Username" sortable />
        <Column field="full_name" header="Name" sortable />
        <Column header="Active" style="width: 6rem">
          <template #body="{ data }">
            <Tag :severity="data.active ? 'success' : 'secondary'" :value="data.active ? 'Yes' : 'No'" />
          </template>
        </Column>
        <Column header="Admin" style="width: 6rem">
          <template #body="{ data }">
            <Tag :severity="data.is_admin ? 'warn' : 'secondary'" :value="data.is_admin ? 'Yes' : 'No'" />
          </template>
        </Column>
        <Column header="" style="width: 8rem">
          <template #body="{ data }">
            <Button icon="pi pi-pencil" text rounded size="small" @click="openEdit(data)" />
            <Button icon="pi pi-trash" text rounded size="small" severity="danger" @click="confirmDelete(data)" />
          </template>
        </Column>
      </DataTable>
    </div>

    <Dialog
      v-model:visible="dialogVisible"
      :header="editing ? 'Edit user' : 'New user'"
      modal
      style="width: min(480px, 95vw)"
    >
      <div class="flex flex-column gap-3">
        <div class="flex flex-column gap-1">
          <label>Username</label>
          <InputText v-model="form.name" class="w-full" autocomplete="off" />
        </div>
        <div class="flex flex-column gap-1">
          <label>Name</label>
          <InputText v-model="form.full_name" class="w-full" autocomplete="name" />
        </div>
        <div class="flex flex-column gap-1">
          <label>Password {{ editing ? '(leave blank to keep)' : '' }}</label>
          <Password
            v-model="form.password"
            :feedback="!editing"
            toggle-mask
            input-class="w-full"
            class="w-full"
            autocomplete="new-password"
          />
        </div>
        <div class="flex align-items-center gap-2">
          <Checkbox v-model="form.active" binary input-id="uactive" />
          <label for="uactive">Active</label>
        </div>
        <div class="flex align-items-center gap-2">
          <Checkbox v-model="form.is_admin" binary input-id="uadmin" />
          <label for="uadmin">Administrator</label>
        </div>
      </div>
      <template #footer>
        <Button label="Cancel" text @click="dialogVisible = false" />
        <Button label="Save" :loading="saving" @click="save" />
      </template>
    </Dialog>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Checkbox from 'primevue/checkbox'
import Tag from 'primevue/tag'
import api, { errorMessage } from '@/api/client'

const toast = useToast()
const confirm = useConfirm()

const items = ref([])
const loading = ref(false)
const saving = ref(false)
const dialogVisible = ref(false)
const editing = ref(null)
const form = reactive({ name: '', full_name: '', password: '', active: true, is_admin: false })

async function load() {
  loading.value = true
  try {
    const { data } = await api.get('/users')
    items.value = data.data || []
  } catch (err) {
    toast.add({ severity: 'error', summary: 'Error', detail: errorMessage(err), life: 4000 })
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  form.name = ''
  form.full_name = ''
  form.password = ''
  form.active = true
  form.is_admin = false
  dialogVisible.value = true
}

function openEdit(row) {
  editing.value = row
  form.name = row.name
  form.full_name = row.full_name || ''
  form.password = ''
  form.active = !!row.active
  form.is_admin = !!row.is_admin
  dialogVisible.value = true
}

async function save() {
  if (!form.name.trim()) {
    toast.add({ severity: 'warn', summary: 'Validation', detail: 'Username is required', life: 3000 })
    return
  }
  if (!editing.value && form.password.length < 6) {
    toast.add({ severity: 'warn', summary: 'Validation', detail: 'Password must be at least 6 characters', life: 3000 })
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      const body = {
        name: form.name.trim(),
        full_name: form.full_name.trim(),
        active: form.active,
        is_admin: form.is_admin,
      }
      if (form.password) body.password = form.password
      await api.put(`/users/${editing.value.id}`, body)
    } else {
      await api.post('/users', {
        name: form.name.trim(),
        full_name: form.full_name.trim(),
        password: form.password,
        active: form.active,
        is_admin: form.is_admin,
      })
    }
    dialogVisible.value = false
    await load()
    toast.add({ severity: 'success', summary: 'Saved', life: 2000 })
  } catch (err) {
    toast.add({ severity: 'error', summary: 'Error', detail: errorMessage(err), life: 4000 })
  } finally {
    saving.value = false
  }
}

function confirmDelete(row) {
  confirm.require({
    message: `Delete user "${row.full_name || row.name}"?`,
    header: 'Confirm',
    icon: 'pi pi-exclamation-triangle',
    acceptClass: 'p-button-danger',
    accept: async () => {
      try {
        await api.delete(`/users/${row.id}`)
        await load()
        toast.add({ severity: 'success', summary: 'Deleted', life: 2000 })
      } catch (err) {
        toast.add({ severity: 'error', summary: 'Error', detail: errorMessage(err), life: 4000 })
      }
    },
  })
}

onMounted(load)
</script>
