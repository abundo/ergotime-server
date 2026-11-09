<template>
    <div class="layout-shell">
        <header class="layout-topbar">
            <div class="brand">Ergotime</div>
            <nav>
                <RouterLink :to="{ name: 'dashboard' }">Dashboard</RouterLink>
                <RouterLink :to="{ name: 'customers' }">Customers</RouterLink>
                <RouterLink :to="{ name: 'reports' }">Reports</RouterLink>
                <RouterLink :to="{ name: 'monthly' }">Monthly</RouterLink>
                <RouterLink :to="{ name: 'timesheets' }">Timesheets</RouterLink>
                <RouterLink v-if="auth.isAdmin" :to="{ name: 'users' }">Users</RouterLink>
            </nav>
            <div class="user-area">
                <RouterLink :to="{ name: 'profile' }">{{ auth.displayName }}</RouterLink>
                <Button label="Logout" severity="secondary" size="small" text @click="onLogout" />
            </div>
        </header>
        <main class="layout-main">
            <RouterView />
        </main>
    </div>
</template>

<script setup>
import { useRouter } from "vue-router";
import Button from "primevue/button";
import { useAuthStore } from "@/stores/auth";

const auth = useAuthStore();
const router = useRouter();

async function onLogout() {
    await auth.logout();
    router.push({ name: "login" });
}
</script>
