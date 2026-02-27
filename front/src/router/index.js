import { createRouter, createWebHistory } from "vue-router";

const LoginView = () => import("../views/LoginView.vue");
const AdminLayout = () => import("../views/AdminLayout.vue");
const UserLayout = () => import("../views/UserLayout.vue");
const ModelsView = () => import("../views/ModelsView.vue");
const CombosView = () => import("../views/CombosView.vue");
const OperatorsView = () => import("../views/OperatorsView.vue");
const UsersView = () => import("../views/UsersView.vue");
const MyUsageView = () => import("../views/MyUsageView.vue");
const ErrorLogsView = () => import("../views/ErrorLogsView.vue");

const routes = [
  {
    path: "/",
    redirect: "/login",
  },
  {
    path: "/login",
    component: LoginView,
  },
  {
    path: "/",
    component: AdminLayout,
    meta: { requiresAdmin: true },
    children: [
      { path: "models", component: ModelsView },
      { path: "operators", component: OperatorsView },
      { path: "combos", component: CombosView },
      { path: "users", component: UsersView },
      { path: "api-test", component: () => import("@/views/ApiTestView.vue") },
      { path: "error-logs", component: ErrorLogsView },
    ],
  },
  {
    path: "/",
    component: UserLayout,
    meta: { requiresUser: true },
    children: [{ path: "my-usage", component: MyUsageView }],
  },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

// 路由守卫：根据 is_admin 跳转到对应布局
router.beforeEach((to, from, next) => {
  const isAdmin = localStorage.getItem("is_admin") === "1";
  const token = localStorage.getItem("token");

  // 未登录只能访问登录页
  if (!token && to.path !== "/login") {
    next("/login");
    return;
  }

  // 已登录访问登录页，跳转到对应首页
  if (to.path === "/login" && token) {
    next(isAdmin ? "/models" : "/my-usage");
    return;
  }

  next();
});

export default router;
