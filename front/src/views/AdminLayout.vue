<script setup>
import { ref } from 'vue';
import Sidebar from '@/components/Sidebar.vue';

defineProps({
  isAdmin: {
    type: Boolean,
    default: true
  }
});

const isMobile = ref(window.innerWidth <= 768);
window.addEventListener('resize', () => {
  isMobile.value = window.innerWidth <= 768;
});

const sidebarVisible = ref(false);
</script>

<template>
  <div class="layout-container">
    <!-- 动态背景特效 -->
    <div class="bg-orb orb-1"></div>
    <div class="bg-orb orb-2"></div>
    <div class="bg-orb orb-3"></div>

    <el-container class="main-layout">
      <!-- 移动端侧边栏切换按钮 -->
      <div v-if="isMobile" class="mobile-header glass-panel">
        <div class="logo-text">ClaudeRouter</div>
        <el-button @click="sidebarVisible = !sidebarVisible" color="rgba(255,255,255,0.1)" class="menu-btn">
          <el-icon><Menu /></el-icon>
        </el-button>
      </div>

      <!-- 侧边栏 (PC 端常驻, 移动端抽屉) -->
      <template v-if="isMobile">
        <el-drawer
          v-model="sidebarVisible"
          direction="ltr"
          :with-header="false"
          size="220px"
          class="mobile-sidebar-drawer"
        >
          <Sidebar :is-admin="isAdmin" @menu-click="sidebarVisible = false" />
        </el-drawer>
      </template>
      <template v-else>
        <Sidebar :is-admin="isAdmin" />
      </template>

      <!-- 右侧主体内容 -->
      <el-container class="content-wrapper">
        <el-main class="main-area">
          <div class="content-inner glass-panel">
            <router-view />
          </div>
        </el-main>
      </el-container>
    </el-container>
  </div>
</template>

<style scoped>
.layout-container {
  min-height: 100vh;
  width: 100%;
  background-color: #050b14;
  background-image: radial-gradient(circle at 15% 50%, rgba(20, 30, 60, 0.4), transparent 50%),
                    radial-gradient(circle at 85% 30%, rgba(30, 20, 60, 0.4), transparent 50%);
  position: relative;
  overflow: hidden;
  font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  color: #e2e8f0;
}

.bg-orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
  z-index: 0;
  opacity: 0.5;
  pointer-events: none;
}
.orb-1 {
  width: 400px;
  height: 400px;
  background: rgba(102, 126, 234, 0.3);
  top: -100px;
  left: -100px;
  animation: float 10s ease-in-out infinite;
}
.orb-2 {
  width: 350px;
  height: 350px;
  background: rgba(118, 75, 162, 0.3);
  bottom: 10%;
  right: -50px;
  animation: float 12s ease-in-out infinite reverse;
}
.orb-3 {
  width: 250px;
  height: 250px;
  background: rgba(16, 185, 129, 0.2);
  top: 40%;
  left: 30%;
  animation: float 8s ease-in-out infinite 2s;
}

@keyframes float {
  0%, 100% { transform: translateY(0) scale(1); }
  50% { transform: translateY(-30px) scale(1.05); }
}

.main-layout {
  position: relative;
  z-index: 10;
  height: 100vh;
  display: flex;
}

.mobile-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 20px;
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  z-index: 40;
  border-radius: 0;
  border-bottom: 1px solid rgba(255,255,255,0.08);
}
.logo-text {
  font-size: 20px;
  font-weight: 800;
  background: linear-gradient(135deg, #fff 0%, #a5b4fc 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}
.menu-btn {
  border: none;
  font-size: 20px;
  padding: 8px;
}

.content-wrapper {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.main-area {
  padding: 24px;
  overflow-y: auto;
  height: 100%;
}

.content-inner {
  min-height: 100%;
  border-radius: 16px;
  padding: 0;
  animation: fadeIn 0.4s ease-out;
}

.glass-panel {
  background: rgba(15, 23, 42, 0.6);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, 0.08);
  box-shadow: 0 10px 30px -10px rgba(0, 0, 0, 0.5);
}

@keyframes fadeIn {
  from { opacity: 0; transform: translateY(10px); }
  to { opacity: 1; transform: translateY(0); }
}

@media (max-width: 768px) {
  .main-layout {
    flex-direction: column;
  }
  .main-area {
    padding: 16px;
    padding-top: 76px; /* leave space for mobile header */
  }
}
</style>
<style>
.mobile-sidebar-drawer .el-drawer__body {
  padding: 0;
  background: #0f172a;
}
</style>
