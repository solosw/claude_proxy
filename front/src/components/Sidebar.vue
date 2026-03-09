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

const emit = defineEmits(['menu-click']);

const menuItems = computed(() => {
  if (props.isAdmin) {
    return [
      { path: '/models', label: '模型管理', icon: 'Setting' },
      { path: '/users', label: '用户管理', icon: 'User' },
      { path: '/operators', label: '运营商', icon: 'Connection' },
      { path: '/redeem-codes', label: '兑换码', icon: 'Grid' },
      { path: '/combos', label: '组合模型', icon: 'Grid' },
      { path: '/error-logs', label: '错误日志', icon: 'Warning' },
      { path: '/api-test', label: '测试', icon: 'Promotion' },
    ];
  }
  return [];
});

const handleMenuSelect = (path) => {
  if (path && path !== route.path) {
    router.push(path);
    emit('menu-click');
  }
};

const handleLogout = () => {
  ElMessageBox.confirm('确定要退出登录吗？', '提示', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    type: 'warning',
    customClass: 'dark-message-box'
  }).then(() => {
    localStorage.removeItem('token');
    localStorage.removeItem('is_admin');
    localStorage.removeItem('username');
    router.push('/login');
  }).catch(() => {});
};
</script>

<template>
  <el-aside width="220px" class="sidebar glass-panel">
    <div class="logo">
      <span class="logo-text">ClaudeRouter</span>
    </div>
    <div class="menu-container">
      <el-menu
        :default-active="route.path"
        router
        @select="handleMenuSelect"
        class="custom-menu"
      >
        <el-menu-item
          v-for="item in menuItems"
          :key="item.path"
          :index="item.path"
          class="custom-menu-item"
        >
          <el-icon><component :is="item.icon" /></el-icon>
          <span>{{ item.label }}</span>
        </el-menu-item>
      </el-menu>
    </div>

    <div class="logout-section">
      <el-button
        color="#ef4444" 
        plain
        :icon="'SwitchButton'"
        @click="handleLogout"
        class="logout-btn"
      >
        退出登录
      </el-button>
    </div>
  </el-aside>
</template>

<style scoped>
.sidebar {
  background: rgba(15, 23, 42, 0.4) !important;
  backdrop-filter: blur(24px);
  border-right: 1px solid rgba(255, 255, 255, 0.05) !important;
  box-shadow: 4px 0 20px rgba(0, 0, 0, 0.3);
  display: flex;
  flex-direction: column;
  height: 100vh;
  z-index: 30;
}

.logo {
  height: 76px;
  display: flex;
  align-items: center;
  padding: 0 24px;
  font-weight: 800;
  font-size: 22px;
  letter-spacing: -0.5px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.logo-text {
  background: linear-gradient(135deg, #fff 0%, #a5b4fc 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}

.menu-container {
  flex: 1;
  overflow-y: auto;
  padding: 16px 12px;
}

/* 隐藏滚动条 */
.menu-container::-webkit-scrollbar {
  width: 4px;
}
.menu-container::-webkit-scrollbar-thumb {
  background: rgba(255,255,255,0.1);
  border-radius: 4px;
}

.custom-menu {
  background: transparent !important;
  border-right: none !important;
}

.custom-menu-item {
  color: #94a3b8 !important;
  margin-bottom: 8px;
  border-radius: 12px;
  transition: all 0.3s ease;
  height: 48px;
  line-height: 48px;
}

.custom-menu-item:hover {
  background: rgba(255, 255, 255, 0.05) !important;
  color: #f1f5f9 !important;
  transform: translateX(4px);
}

.custom-menu-item.is-active {
  background: linear-gradient(90deg, rgba(99, 102, 241, 0.15) 0%, rgba(139, 92, 246, 0.15) 100%) !important;
  color: #a5b4fc !important;
  border: 1px solid rgba(99, 102, 241, 0.3);
  box-shadow: inset 4px 0 0 #8b5cf6;
}
.custom-menu-item.is-active .el-icon {
  color: #a5b4fc;
}

.logout-section {
  padding: 24px;
  border-top: 1px solid rgba(255, 255, 255, 0.05);
}

.logout-btn {
  width: 100%;
  border-radius: 10px;
  font-weight: 600;
  height: 40px;
  border: 1px solid rgba(239, 68, 68, 0.3);
  background: rgba(239, 68, 68, 0.1) !important;
}
.logout-btn:hover {
  background: rgba(239, 68, 68, 0.2) !important;
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(239, 68, 68, 0.2);
}

/* 移动端适配 */
@media (max-width: 768px) {
  .sidebar {
    width: 220px !important;
    position: relative;
    border-right: none !important;
    background: transparent !important;
  }
}
</style>
