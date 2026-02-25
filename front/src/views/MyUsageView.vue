<script setup>
import { onMounted, ref } from 'vue';
import axios from 'axios';
import { ElMessage, ElMessageBox } from 'element-plus';
import { useRouter } from 'vue-router';

const router = useRouter();
const loading = ref(false);
const usage = ref(null);
const usageLogs = ref([]);
const logsLoading = ref(false);
const currentPage = ref(1);
const pageSize = ref(10);
const total = ref(0);

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

const loadUsageLogs = async () => {
  logsLoading.value = true;
  try {
    const { data } = await axios.get('/api/me/usage/logs', {
      params: {
        page: currentPage.value,
        page_size: pageSize.value,
      },
    });
    usageLogs.value = data.logs || [];
    total.value = data.total || 0;
  } catch (_) {
    usageLogs.value = [];
    ElMessage.error('加载使用日志失败');
  } finally {
    logsLoading.value = false;
  }
};

const handlePageChange = (page) => {
  currentPage.value = page;
  loadUsageLogs();
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

const copyToClipboard = (text) => {
  navigator.clipboard.writeText(text).then(() => {
    ElMessage.success('已复制到剪贴板');
  }).catch(() => {
    ElMessage.error('复制失败');
  });
};

onMounted(() => {
  loadMyUsage();
  loadUsageLogs();
});
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

    <el-skeleton v-if="loading" :rows="4" animated />

    <div v-else-if="usage" class="usage-grid">

      <div class="usage-card">
        <div class="usage-label">用户名</div>
        <div class="usage-value">{{ usage.username }}</div>
      </div>
      <div class="usage-card">
        <div class="usage-label">API Key</div>
        <div class="usage-value api-key-display">
          <span class="api-key-text">{{ usage.api_key }}</span>
          <el-button
            link
            type="primary"
            size="small"
            @click="copyToClipboard(usage.api_key)"
          >
            复制
          </el-button>
        </div>
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

    <div v-if="usage" class="logs-section">
      <h3 class="logs-title">使用日志</h3>
      <el-table
        :data="usageLogs"
        :loading="logsLoading"
        stripe
        style="width: 100%"
        class="usage-logs-table"
      >
        <el-table-column prop="model_id" label="模型名"  />
        <el-table-column prop="provider" label="供应商"  />
        <el-table-column label="消耗总Token"  align="right">
          <template #default="{ row }">
            {{ (row.input_tokens || 0) + (row.output_tokens || 0) }}
          </template>
        </el-table-column>
        <el-table-column prop="total_cost" label="总消耗额度"  align="right">
          <template #default="{ row }">
            {{ row.total_cost ? row.total_cost.toFixed(6) : '0' }}
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="时间"  />
      </el-table>

      <div class="pagination-container">
        <el-pagination
          v-model:current-page="currentPage"
          :page-size="pageSize"
          :total="total"
          layout="total, prev, pager, next, jumper"
          @current-change="handlePageChange"
        />
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

.api-key-display {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px !important;
}

.api-key-text {
  font-family: 'Courier New', monospace;
  font-size: 12px;
  word-break: break-all;
  flex: 1;
}

.logs-section {
  margin-top: 32px;
  padding: 20px;
  background: rgba(255, 255, 255, 0.6);
  backdrop-filter: blur(10px);
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.2);
  box-shadow: 0 4px 15px rgba(102, 126, 234, 0.1);
  animation: fadeInUp 0.6s ease-out 0.2s both;
}

.logs-title {
  font-size: 18px;
  font-weight: 600;
  margin: 0 0 16px;
  color: #111827;
}

.usage-logs-table {
  margin-bottom: 16px;
}

.usage-logs-table :deep(.el-table__header th .cell) {
  color: #303133;
}

.usage-logs-table :deep(.el-table__header th) {
  background-color: rgba(102, 126, 234, 0.05);
  font-weight: 600;
}

.usage-logs-table :deep(.el-table__body tr:hover > td) {
  background-color: rgba(102, 126, 234, 0.08) !important;
}

.pagination-container {
  display: flex;
  justify-content: flex-end;
  padding-top: 16px;
  border-top: 1px solid rgba(0, 0, 0, 0.06);
}

.pagination-container :deep(.el-pagination) {
  display: flex;
  gap: 8px;
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
