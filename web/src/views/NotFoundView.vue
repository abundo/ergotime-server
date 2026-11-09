<template>
  <div class="login-page">
    <div class="login-card" style="text-align: center">
      <h1>404</h1>
      <p class="subtitle">Page not found</p>
      <RouterLink :to="{ name: 'dashboard' }">
        <Button label="Go home" />
      </RouterLink>
    </div>
  </div>
</template>

<script setup>
import Button from 'primevue/button'
</script>
