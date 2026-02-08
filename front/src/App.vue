<script setup>
import { useRoute, useRouter } from 'vue-router';

const route = useRoute();
const router = useRouter();

const handleMenuSelect = (path) => {
  if (path && path !== route.path) {
    router.push(path);
  }
};
</script>

<template>
  <el-container style="height: 100vh;">
    <el-aside width="220px" class="aside">
      <div class="logo">
        ClaudeRouter
      </div>
      <el-menu
        :default-active="route.path"
        router
        @select="handleMenuSelect"
      >
        <el-menu-item index="/models">
          <span>模型管理</span>
        </el-menu-item>
        <el-menu-item index="/operators">
          <span>运营商</span>
        </el-menu-item>
        <el-menu-item index="/combos">
          <span>组合模型</span>
        </el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-main class="main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<style scoped>
.aside {
  background: rgba(30, 30, 46, 0.7) !important;
  backdrop-filter: blur(10px);
  border-right: 1px solid rgba(255, 255, 255, 0.1) !important;
  box-shadow: 4px 0 20px rgba(0, 0, 0, 0.1);
  transition: all 0.3s ease;
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

.main {
  background: transparent !important;
  padding: 24px;
}
</style>

