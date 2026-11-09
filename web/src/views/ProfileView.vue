<template>
  <div>
    <h1 class="page-title">Profile</h1>
    <div class="card-panel" style="max-width: 420px">
      <p><strong>Username:</strong> {{ auth.user?.name }}</p>
      <p v-if="auth.user?.full_name"><strong>Name:</strong> {{ auth.user.full_name }}</p>
      <p><strong>Role:</strong> {{ auth.isAdmin ? 'Administrator' : 'User' }}</p>
      <p><strong>User ID:</strong> {{ auth.user?.id }}</p>

      <h3 class="mt-4 mb-2" style="font-size: 1.05rem">Change password</h3>
      <div class="flex flex-column gap-3">
        <div class="flex flex-column gap-1">
          <label>New password</label>
          <Password
            v-model="password"
            toggle-mask
            :feedback="true"
            input-class="w-full"
            class="w-full"
            autocomplete="new-password"
          />
        </div>
        <div class="flex flex-column gap-1">
          <label>Confirm password</label>
          <Password
            v-model="confirm"
            toggle-mask
            :feedback="false"
            input-class="w-full"
            class="w-full"
            autocomplete="new-password"
          />
        </div>
        <Button label="Update password" :loading="saving" @click="save" style="align-self: flex-start" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import Password from 'primevue/password'
import Button from 'primevue/button'
import api, { errorMessage } from '@/api/client'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const toast = useToast()
const password = ref('')
const confirm = ref('')
const saving = ref(false)

async function save() {
  if (password.value.length < 6) {
    toast.add({ severity: 'warn', summary: 'Validation', detail: 'Password must be at least 6 characters', life: 3000 })
    return
  }
  if (password.value !== confirm.value) {
    toast.add({ severity: 'warn', summary: 'Validation', detail: 'Passwords do not match', life: 3000 })
    return
  }
  saving.value = true
  try {
    await api.put(`/users/${auth.user.id}`, { password: password.value })
    password.value = ''
    confirm.value = ''
    toast.add({ severity: 'success', summary: 'Password updated', life: 2500 })
  } catch (err) {
    toast.add({ severity: 'error', summary: 'Error', detail: errorMessage(err), life: 4000 })
  } finally {
    saving.value = false
  }
}
</script>
