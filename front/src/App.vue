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
  border-right: 1px solid #e5e7eb;
  background-color: #ffffff;
}

.logo {
  height: 56px;
  display: flex;
  align-items: center;
  padding: 0 16px;
  font-weight: 600;
  font-size: 18px;
  border-bottom: 1px solid #e5e7eb;
}

.main {
  background-color: #f3f4f6;
}
</style>

