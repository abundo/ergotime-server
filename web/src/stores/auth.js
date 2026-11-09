import { defineStore } from "pinia";
import { ref, computed } from "vue";
import api, { errorMessage } from "@/api/client";

export const useAuthStore = defineStore("auth", () => {
  const user = ref(null);
  const loading = ref(false);
  const ready = ref(false);

  const isAuthenticated = computed(() => !!user.value);
  const isAdmin = computed(() => !!user.value?.is_admin);
  const displayName = computed(() => user.value?.full_name || user.value?.name || "");

  async function fetchMe() {
    try {
      const { data } = await api.get("/auth/me");
      user.value = data;
      return true;
    } catch {
      user.value = null;
      return false;
    } finally {
      ready.value = true;
    }
  }

  async function login(username, password) {
    loading.value = true;
    try {
      const { data } = await api.post("/auth/login", { username, password });
      user.value = data.user;
      ready.value = true;
      return { ok: true };
    } catch (err) {
      user.value = null;
      return { ok: false, message: errorMessage(err, "Login failed") };
    } finally {
      loading.value = false;
    }
  }

  async function logout() {
    try {
      await api.post("/auth/logout");
    } catch {
      /* ignore */
    }
    user.value = null;
  }

  return {
    user,
    loading,
    ready,
    isAuthenticated,
    isAdmin,
    displayName,
    fetchMe,
    login,
    logout,
  };
});
