import { createRouter, createWebHistory } from "vue-router";
import { useAuthStore } from "@/stores/auth";

const routes = [
  {
    path: "/login",
    name: "login",
    component: () => import("@/views/LoginView.vue"),
    meta: { guest: true },
  },
  {
    path: "/",
    component: () => import("@/layouts/AppLayout.vue"),
    meta: { requiresAuth: true },
    children: [
      {
        path: "",
        name: "dashboard",
        component: () => import("@/views/DashboardView.vue"),
      },
      {
        path: "customers",
        name: "customers",
        component: () => import("@/views/CustomersView.vue"),
      },
      {
        path: "reports",
        name: "reports",
        component: () => import("@/views/ReportsView.vue"),
      },
      {
        path: "monthly",
        name: "monthly",
        component: () => import("@/views/MonthlyReportView.vue"),
      },
      {
        path: "timesheets",
        name: "timesheets",
        component: () => import("@/views/TimesheetsView.vue"),
      },
      {
        path: "users",
        name: "users",
        component: () => import("@/views/UsersView.vue"),
        meta: { admin: true },
      },
      {
        path: "profile",
        name: "profile",
        component: () => import("@/views/ProfileView.vue"),
      },
    ],
  },
  {
    path: "/:pathMatch(.*)*",
    name: "not-found",
    component: () => import("@/views/NotFoundView.vue"),
  },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

router.beforeEach(async (to) => {
  const auth = useAuthStore();
  if (!auth.ready) {
    await auth.fetchMe();
  }

  if (to.meta.requiresAuth && !auth.isAuthenticated) {
    return { name: "login", query: { redirect: to.fullPath } };
  }
  if (to.meta.guest && auth.isAuthenticated) {
    return { name: "dashboard" };
  }
  if (to.meta.admin && !auth.isAdmin) {
    return { name: "dashboard" };
  }
  return true;
});

export default router;
