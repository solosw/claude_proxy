<script setup>
import { onMounted, ref } from 'vue';
import axios from 'axios';
import { ElMessage, ElMessageBox } from 'element-plus';
import { useRouter } from 'vue-router';

const router = useRouter();
const loading = ref(false);
const usage = ref(null);

const loadMyUsage = async () => {
  loading.value = true;
  try {
    const { data } = await axios.get('/api/me/usage');
    usage.value = data;
  } catch (_) {
    usage.value = null;
    ElMessage.error('加载我的使用情况失败');
  } finally {
    loading.value = false;
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

onMounted(loadMyUsage);
</script>

<template>
  <div class="p-6">
    <div class="header-section">
      <div class="header-content">
        <div>
          <h2 class="page-title">我的使用情况</h2>
          <p class="page-description">查看自己的额度、剩余额度与 Token 使用统计。</p>
        </div>
        <el-button type="danger" @click="handleLogout">
          退出登录
        </el-button>
      </div>
    </div>

    <el-skeleton v-if="loading" :rows="6" animated />

    <div v-else-if="usage" class="usage-grid">
      <div class="usage-card">
        <div class="usage-label">输入 Token</div>
        <div class="usage-value">{{ usage.input_tokens }}</div>
      </div>
      <div class="usage-card">
        <div class="usage-label">输出 Token</div>
        <div class="usage-value">{{ usage.output_tokens }}</div>
      </div>
      <div class="usage-card">
        <div class="usage-label">总计 Token</div>
        <div class="usage-value">{{ usage.total_tokens }}</div>
      </div>
      <div class="usage-card">
        <div class="usage-label">额度</div>
        <div class="usage-value">{{ usage.unlimited ? '无限' : usage.quota }}</div>
      </div>
      <div class="usage-card">
        <div class="usage-label">剩余额度</div>
        <div class="usage-value">{{ usage.unlimited ? '无限' : usage.remaining }}</div>
      </div>
      <div class="usage-card">
        <div class="usage-label">过期时间</div>
        <div class="usage-value usage-time">{{ usage.expire_at || '不过期' }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.p-6 {
  animation: fadeInUp 0.6s ease-out;
}

.header-section {
  margin-bottom: 24px;
  padding: 20px;
  background: rgba(255, 255, 255, 0.6);
  backdrop-filter: blur(10px);
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.2);
  box-shadow: 0 4px 15px rgba(102, 126, 234, 0.1);
}

.header-content {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
}

.page-title {
  font-size: 24px;
  font-weight: 700;
  margin: 0 0 12px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.page-description {
  margin: 0;
  font-size: 14px;
  color: #6b7280;
  line-height: 1.6;
}

.usage-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 16px;
}

.usage-card {
  background: rgba(255, 255, 255, 0.8);
  border: 1px solid rgba(255, 255, 255, 0.3);
  border-radius: 12px;
  padding: 16px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
}

.usage-label {
  color: #6b7280;
  font-size: 13px;
}

.usage-value {
  margin-top: 10px;
  font-size: 24px;
  font-weight: 700;
  color: #111827;
  word-break: break-all;
}

.usage-time {
  font-size: 15px;
}

@keyframes fadeInUp {
  from {
    opacity: 0;
    transform: translateY(20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
