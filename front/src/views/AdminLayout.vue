<script setup>
import Sidebar from '@/components/Sidebar.vue';

defineProps({
  isAdmin: {
    type: Boolean,
    default: true
  }
});
</script>

<template>
  <el-container style="height: 100vh;">
    <Sidebar :is-admin="isAdmin" />
    <el-container>
      <el-main class="main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<style scoped>
.main {
  background: transparent !important;
  padding: 24px;
}
</style>
