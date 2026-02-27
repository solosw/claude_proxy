<script setup>
import { computed } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { ElMessageBox } from 'element-plus';

const route = useRoute();
const router = useRouter();

const props = defineProps({
  isAdmin: {
    type: Boolean,
    default: false
  }
});

const menuItems = computed(() => {
  if (props.isAdmin) {
    return [
      { path: '/models', label: '模型管理', icon: 'Setting' },
      { path: '/users', label: '用户管理', icon: 'User' },
      { path: '/operators', label: '运营商', icon: 'Connection' },
      { path: '/combos', label: '组合模型', icon: 'Grid' },
      { path: '/error-logs', label: '错误日志', icon: 'Warning' },
      { path: '/api-test', label: '测试', icon: 'Promotion' },
    ];
  }
  return [
    { path: '/my-usage', label: '我的使用情况', icon: 'DataAnalysis' },
  ];
});

const handleMenuSelect = (path) => {
  if (path && path !== route.path) {
    router.push(path);
  }
};

const handleLogout = () => {
  ElMessageBox.confirm('确定要退出登录吗？', '提示', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    type: 'warning',
  }).then(() => {
    localStorage.removeItem('token');
    localStorage.removeItem('is_admin');
    localStorage.removeItem('username');
    router.push('/login');
  }).catch(() => {});
};
</script>

<template>
  <el-aside width="220px" class="sidebar">
    <div class="logo">
      ClaudeRouter
    </div>
    <el-menu
      :default-active="route.path"
      router
      @select="handleMenuSelect"
    >
      <el-menu-item
        v-for="item in menuItems"
        :key="item.path"
        :index="item.path"
      >
        <span>{{ item.label }}</span>
      </el-menu-item>
    </el-menu>

    <div class="logout-section">
      <el-button
        type="danger"
        :icon="'SwitchButton'"
        @click="handleLogout"
        style="width: 100%;"
      >
        退出登录
      </el-button>
    </div>
  </el-aside>
</template>

<style scoped>
.sidebar {
  background: rgba(30, 30, 46, 0.7) !important;
  backdrop-filter: blur(10px);
  border-right: 1px solid rgba(255, 255, 255, 0.1) !important;
  box-shadow: 4px 0 20px rgba(0, 0, 0, 0.1);
  transition: all 0.3s ease;
  display: flex;
  flex-direction: column;
}

.logo {
  height: 64px;
  display: flex;
  align-items: center;
  padding: 0 20px;
  font-weight: 700;
  font-size: 20px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
  letter-spacing: 1px;
  transition: all 0.3s ease;
  cursor: pointer;
}

.logo:hover {
  transform: scale(1.05);
  filter: drop-shadow(0 0 8px rgba(102, 126, 234, 0.4));
}

.logout-section {
  margin-top: auto;
  padding: 16px;
  border-top: 1px solid rgba(255, 255, 255, 0.1);
}
</style>
