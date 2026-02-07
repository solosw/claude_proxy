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
  background: linear-gradient(135deg, #eef2ff, #f9fafb);
}

.login-card {
  width: 360px;
  padding: 32px 28px 24px;
  border-radius: 16px;
  background-color: #ffffff;
  box-shadow: 0 18px 45px rgba(15, 23, 42, 0.12);
}

.title {
  margin: 0 0 8px;
  font-size: 22px;
  font-weight: 600;
  color: #111827;
}

.subtitle {
  margin: 0 0 20px;
  font-size: 13px;
  color: #6b7280;
}

.hint {
  margin-top: 8px;
  font-size: 12px;
  color: #9ca3af;
}
</style>

