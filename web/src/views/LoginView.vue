<template>
  <div class="login-page">
    <div class="login-card">
      <h1>Ergotime</h1>
      <p class="subtitle">Sign in to continue</p>

      <form @submit.prevent="onSubmit">
        <div class="flex flex-column gap-3">
          <div class="flex flex-column gap-1">
            <label for="username">Username</label>
            <InputText
              id="username"
              v-model="username"
              autocomplete="username"
              autofocus
              class="w-full"
              :disabled="auth.loading"
            />
          </div>
          <div class="flex flex-column gap-1">
            <label for="password">Password</label>
            <Password
              id="password"
              v-model="password"
              :feedback="false"
              toggle-mask
              input-class="w-full"
              class="w-full"
              autocomplete="current-password"
              :disabled="auth.loading"
            />
          </div>
          <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
          <Button
            type="submit"
            label="Sign in"
            class="w-full"
            :loading="auth.loading"
          />
        </div>
      </form>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Button from 'primevue/button'
import Message from 'primevue/message'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const username = ref('')
const password = ref('')
const error = ref('')

async function onSubmit() {
  error.value = ''
  // Basic client-side validation; never render unsanitized HTML from server
  if (!username.value.trim() || !password.value) {
    error.value = 'Username and password are required'
    return
  }
  const result = await auth.login(username.value.trim(), password.value)
  if (!result.ok) {
    error.value = result.message
    return
  }
  const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
  router.replace(redirect)
}
</script>
