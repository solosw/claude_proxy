<script setup>
import { ref } from 'vue';
import axios from 'axios';
import { ElMessage } from 'element-plus';
import { useRouter } from 'vue-router';
import { Key, Right } from '@element-plus/icons-vue';

const router = useRouter();

const apiKey = ref(localStorage.getItem('token') || '');
const loading = ref(false);

const handleLogin = async () => {
  if (!apiKey.value) {
    ElMessage.error('请输入 API Key');
    return;
  }
  loading.value = true;
  try {
    const { data } = await axios.post('/api/login', {
      api_key: apiKey.value,
    });

    if (data.success) {
      localStorage.setItem('token', apiKey.value);
      localStorage.setItem('is_admin', data.is_admin ? '1' : '0');
      localStorage.setItem('username', data.username || '');
      ElMessage.success('登录成功');
      router.push(data.is_admin ? '/models' : '/my-usage');
    } else {
      ElMessage.error(data.message || '登录失败');
    }
  } catch (e) {
    const msg = e.response?.data?.error || 'API Key 无效或请求失败';
    ElMessage.error(msg);
  } finally {
    loading.value = false;
  }
};
</script>

<template>
  <div class="login-page">
    <div class="bg-orb orb-1"></div>
    <div class="bg-orb orb-2"></div>
    
    <div class="login-wrapper">
      <div class="login-card glass-panel">
        <div class="logo-box">
          <div class="logo-icon"><el-icon><Right /></el-icon></div>
        </div>
        
        <h1 class="title">ClaudeRouter</h1>
        <p class="subtitle">Enter your API Key to continue</p>
        
        <el-form @submit.prevent="handleLogin" class="login-form">
          <el-form-item>
            <el-input
              v-model="apiKey"
              placeholder="Your API Key"
              show-password
              clearable
              class="custom-input login-input"
              size="large"
            >
              <template #prefix>
                <el-icon><Key /></el-icon>
              </template>
            </el-input>
          </el-form-item>

          <el-form-item class="mt-8">
            <el-button
              type="primary"
              :loading="loading"
              class="login-btn glow-btn w-full"
              size="large"
              @click="handleLogin"
            >
              Sign In
            </el-button>
          </el-form-item>
        </el-form>
      </div>
      
      <p class="footer-hint">Protected by Advanced Security. Authorized Access Only.</p>
    </div>
  </div>
</template>

<style scoped>
* { box-sizing: border-box; }
.w-full { width: 100%; }
.mt-8 { margin-top: 32px; }

.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: #050b14;
  background-image: radial-gradient(circle at 50% 50%, rgba(20, 30, 60, 0.6), transparent 80%);
  position: relative;
  overflow: hidden;
  font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}

.bg-orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
  z-index: 0;
  opacity: 0.6;
}
.orb-1 {
  width: 500px;
  height: 500px;
  background: rgba(102, 126, 234, 0.2);
  top: -150px;
  left: -100px;
  animation: float 10s ease-in-out infinite;
}
.orb-2 {
  width: 400px;
  height: 400px;
  background: rgba(139, 92, 246, 0.2);
  bottom: -100px;
  right: -50px;
  animation: float 12s ease-in-out infinite reverse;
}
@keyframes float {
  0%, 100% { transform: translateY(0) scale(1); }
  50% { transform: translateY(-30px) scale(1.05); }
}

.login-wrapper {
  position: relative;
  z-index: 10;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 24px;
  width: 100%;
  padding: 0 24px;
}

.login-card {
  width: 100%;
  max-width: 420px;
  padding: 48px 40px;
  border-radius: 24px;
  display: flex;
  flex-direction: column;
  align-items: center;
  animation: fadeInUp 0.6s cubic-bezier(0.16, 1, 0.3, 1);
}

.glass-panel {
  background: rgba(15, 23, 42, 0.5);
  backdrop-filter: blur(24px);
  -webkit-backdrop-filter: blur(24px);
  border: 1px solid rgba(255, 255, 255, 0.08);
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5), inset 0 1px 0 rgba(255,255,255,0.1);
  transition: transform 0.3s ease, box-shadow 0.3s ease;
}
.glass-panel:hover {
  box-shadow: 0 30px 60px -15px rgba(0, 0, 0, 0.6), inset 0 1px 0 rgba(255,255,255,0.15), 0 0 40px rgba(99, 102, 241, 0.15);
}

.logo-box {
  width: 64px;
  height: 64px;
  border-radius: 16px;
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.2), rgba(139, 92, 246, 0.1));
  border: 1px solid rgba(139, 92, 246, 0.4);
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 24px;
  box-shadow: 0 10px 20px rgba(0,0,0,0.2), inset 0 0 20px rgba(139, 92, 246, 0.2);
}
.logo-icon {
  font-size: 32px;
  color: #a5b4fc;
}

.title {
  margin: 0 0 8px;
  font-size: 32px;
  font-weight: 800;
  text-align: center;
  background: linear-gradient(135deg, #ffffff 0%, #a5b4fc 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
  letter-spacing: -0.5px;
}

.subtitle {
  margin: 0 0 32px;
  font-size: 15px;
  color: #94a3b8;
  text-align: center;
}

.login-form {
  width: 100%;
}

.login-input :deep(.el-input__wrapper) {
  background: rgba(0,0,0,0.3) !important;
  box-shadow: 0 0 0 1px rgba(255,255,255,0.1) inset !important;
  border-radius: 12px;
  padding: 8px 16px;
  transition: all 0.3s ease;
}
.login-input :deep(.el-input__wrapper:hover), .login-input :deep(.el-input__wrapper.is-focus) {
  box-shadow: 0 0 0 1px #a5b4fc inset !important;
  background: rgba(0,0,0,0.4) !important;
}
.login-input :deep(.el-input__inner) {
  color: #f8fafc;
  font-family: monospace;
}
.login-input :deep(.el-icon) {
  color: #94a3b8;
  font-size: 18px;
}

.glow-btn {
  background: linear-gradient(135deg, #6366f1, #8b5cf6) !important;
  border: none !important;
  color: white !important;
  border-radius: 12px !important;
  font-weight: 600 !important;
  letter-spacing: 0.5px;
  box-shadow: 0 8px 20px rgba(99, 102, 241, 0.3) !important;
  transition: all 0.3s ease !important;
  height: 48px;
}
.glow-btn:hover {
  transform: translateY(-2px);
  box-shadow: 0 12px 25px rgba(99, 102, 241, 0.5) !important;
  background: linear-gradient(135deg, #4f46e5, #7c3aed) !important;
}

.footer-hint {
  font-size: 13px;
  color: #64748b;
  text-align: center;
  opacity: 0;
  animation: fadeIn 1s ease 1s forwards;
}

@keyframes fadeInUp {
  from { opacity: 0; transform: translateY(40px); }
  to { opacity: 1; transform: translateY(0); }
}
@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}
</style>
