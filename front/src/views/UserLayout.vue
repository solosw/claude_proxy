<script setup>
</script>

<template>
  <div class="layout-container">
    <div class="bg-orb orb-1"></div>
    <div class="bg-orb orb-2"></div>
    <div class="bg-orb orb-3"></div>
    
    <el-container class="main-layout">
      <el-main class="main-area">
        <router-view />
      </el-main>
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
.orb-1 { width: 400px; height: 400px; background: rgba(102, 126, 234, 0.3); top: -100px; left: -100px; animation: float 10s ease-in-out infinite; }
.orb-2 { width: 350px; height: 350px; background: rgba(118, 75, 162, 0.3); bottom: 10%; right: -50px; animation: float 12s ease-in-out infinite reverse; }
.orb-3 { width: 250px; height: 250px; background: rgba(16, 185, 129, 0.2); top: 40%; left: 30%; animation: float 8s ease-in-out infinite 2s; }

@keyframes float {
  0%, 100% { transform: translateY(0) scale(1); }
  50% { transform: translateY(-30px) scale(1.05); }
}

.main-layout {
  position: relative;
  z-index: 10;
  height: 100vh;
}

.main-area {
  padding: 0;
  margin: 0;
  height: 100%;
  overflow-y: auto;
}
</style>
