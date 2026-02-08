<script setup>
import { ref } from 'vue';
import axios from 'axios';
import { ElMessage } from 'element-plus';
import { useRouter } from 'vue-router';

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
    // 使用用户输入的 API Key 访问一个受保护的接口来校验
    await axios.get('/api/models', {
      headers: {
        'X-API-Key': apiKey.value,
      },
    });
    localStorage.setItem('token', apiKey.value);
    ElMessage.success('登录成功');
    router.push('/models');
  } catch (e) {
    ElMessage.error('API Key 无效或请求失败');
  } finally {
    loading.value = false;
  }
};
</script>

<template>
  <div class="login-page">
    <div class="login-card">
      <h1 class="title">ClaudeRouter 登录</h1>
      <p class="subtitle">
        请输入配置在后端 <code>config.yaml</code> 中的 API Key
      </p>

      <el-form @submit.prevent="handleLogin">
        <el-form-item label="API Key">
          <el-input
            v-model="apiKey"
            placeholder="请输入 API Key"
            show-password
            clearable
          />
        </el-form-item>

        <el-form-item>
          <el-button
            type="primary"
            :loading="loading"
            style="width: 100%;"
            @click="handleLogin"
          >
            登录
          </el-button>
        </el-form-item>
      </el-form>

      <p class="hint">
        后续在 Claude Code 中配置该 API Key，即可通过本中转站调用不同模型服务商。
      </p>
    </div>
  </div>
</template>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 50%, #f093fb 100%);
  background-attachment: fixed;
  position: relative;
  overflow: hidden;
}

/* 背景装饰元素 */
.login-page::before {
  content: '';
  position: absolute;
  width: 400px;
  height: 400px;
  background: radial-gradient(circle, rgba(240, 147, 251, 0.2) 0%, transparent 70%);
  border-radius: 50%;
  top: -100px;
  left: -100px;
  animation: float 6s ease-in-out infinite;
}

.login-page::after {
  content: '';
  position: absolute;
  width: 300px;
  height: 300px;
  background: radial-gradient(circle, rgba(79, 172, 254, 0.2) 0%, transparent 70%);
  border-radius: 50%;
  bottom: -50px;
  right: -50px;
  animation: float 8s ease-in-out infinite reverse;
}

.login-card {
  width: 360px;
  padding: 40px 32px;
  border-radius: 20px;
  background: rgba(255, 255, 255, 0.8);
  backdrop-filter: blur(10px);
  border: 1px solid rgba(255, 255, 255, 0.2);
  box-shadow: 0 8px 32px 0 rgba(31, 38, 135, 0.2);
  position: relative;
  z-index: 10;
  animation: fadeInUp 0.6s ease-out;
  transition: all 0.3s ease;
}

.login-card:hover {
  transform: translateY(-8px);
  box-shadow: 0 12px 40px rgba(102, 126, 234, 0.3);
  border-color: rgba(255, 255, 255, 0.3);
}

.title {
  margin: 0 0 12px;
  font-size: 28px;
  font-weight: 700;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
  letter-spacing: 0.5px;
}

.subtitle {
  margin: 0 0 24px;
  font-size: 14px;
  color: #6b7280;
  line-height: 1.6;
}

.subtitle code {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
  font-weight: 600;
  font-family: 'Monaco', 'Menlo', monospace;
}

.hint {
  margin-top: 16px;
  font-size: 13px;
  color: #9ca3af;
  line-height: 1.6;
  padding: 12px;
  background: rgba(102, 126, 234, 0.05);
  border-left: 3px solid #667eea;
  border-radius: 6px;
}

@keyframes fadeInUp {
  from {
    opacity: 0;
    transform: translateY(30px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes float {
  0%, 100% {
    transform: translateY(0px);
  }
  50% {
    transform: translateY(-20px);
  }
}
</style>

