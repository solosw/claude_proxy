import { createRouter, createWebHistory } from 'vue-router';

const LoginView = () => import('../views/LoginView.vue');
const ModelsView = () => import('../views/ModelsView.vue');
const CombosView = () => import('../views/CombosView.vue');
const OperatorsView = () => import('../views/OperatorsView.vue');

const routes = [
  {
    path: '/',
    redirect: '/login',
  },
  {
    path: '/login',
    component: LoginView,
  },
  {
    path: '/models',
    component: ModelsView,
  },
  {
    path: '/operators',
    component: OperatorsView,
  },
  {
    path: '/combos',
    component: CombosView,
  },
  {
    path: '/api-test',
    name: 'ApiTest',
    component: () => import('@/views/ApiTestView.vue')
  }

];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

export default router;


