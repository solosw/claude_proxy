<script setup>
import { onMounted, ref } from 'vue';
import axios from 'axios';
import { ElMessage } from 'element-plus';

const loading = ref(false);
const logs = ref([]);
const total = ref(0);
const models = ref([]);

const filterModelID = ref('');
const page = ref(1);
const pageSize = ref(20);

const loadModels = async () => {
  try {
    const { data } = await axios.get('/api/models');
    models.value = data || [];
  } catch (_) {}
};

const loadLogs = async () => {
  loading.value = true;
  try {
    const params = {
      page: page.value,
      page_size: pageSize.value,
    };
    if (filterModelID.value) params.model_id = filterModelID.value;
    const { data } = await axios.get('/api/error-logs', { params });
    logs.value = data.logs || [];
    total.value = data.total || 0;
  } catch (_) {
    ElMessage.error('加载错误日志失败');
  } finally {
    loading.value = false;
  }
};

const handleFilterChange = () => {
  page.value = 1;
  loadLogs();
};

const handlePageChange = (p) => {
  page.value = p;
  loadLogs();
};

const handlePageSizeChange = (ps) => {
  pageSize.value = ps;
  page.value = 1;
  loadLogs();
};

const resetFilter = () => {
  filterModelID.value = '';
  page.value = 1;
  loadLogs();
};

const statusTagType = (code) => {
  if (code === 0) return 'info';
  if (code >= 500) return 'danger';
  if (code >= 400) return 'warning';
  return 'success';
};

onMounted(() => {
  loadModels();
  loadLogs();
});
</script>

<template>
  <div class="p-6">
    <div class="flex items-center justify-between mb-4">
      <h2 class="text-xl font-semibold">错误日志</h2>
      <el-button @click="loadLogs">刷新</el-button>
    </div>

    <!-- 筛选 -->
    <div class="filter-panel mb-4">
      <el-form layout="inline" class="filter-form">
        <el-form-item label="模型 ID">
          <el-select
            v-model="filterModelID"
            placeholder="全部模型"
            clearable
            filterable
            style="width: 260px"
            @change="handleFilterChange"
          >
            <el-option
              v-for="m in models"
              :key="m.id"
              :label="`${m.name || m.id} (${m.id})`"
              :value="m.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button @click="resetFilter">重置</el-button>
        </el-form-item>
      </el-form>
    </div>

    <el-table v-loading="loading" :data="logs" border style="width: 100%">
      <el-table-column prop="id" label="ID" width="80" />
      <el-table-column prop="model_id" label="模型 ID" width="220" />
      <el-table-column prop="username" label="用户" width="160" />
      <el-table-column prop="status_code" label="状态码" width="100">
        <template #default="{ row }">
          <el-tag :type="statusTagType(row.status_code)" size="small">
            {{ row.status_code || '-' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="error_msg" label="错误信息" min-width="300">
        <template #default="{ row }">
          <span class="error-msg">{{ row.error_msg }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="created_at" label="时间" width="180">
        <template #default="{ row }">
          {{ row.created_at ? new Date(row.created_at).toLocaleString() : '-' }}
        </template>
      </el-table-column>
    </el-table>

    <div class="pagination-bar">
      <el-pagination
        v-model:current-page="page"
        v-model:page-size="pageSize"
        :total="total"
        :page-sizes="[20, 50, 100]"
        layout="total, sizes, prev, pager, next"
        @current-change="handlePageChange"
        @size-change="handlePageSizeChange"
      />
    </div>
  </div>
</template>

<style scoped>
.p-6 {
  animation: fadeInUp 0.6s ease-out;
}

.filter-panel {
  background: #f5f7fa;
  padding: 12px;
  border-radius: 4px;
}

.filter-form {
  display: flex;
  gap: 12px;
  align-items: center;
}

.error-msg {
  font-family: 'Courier New', monospace;
  font-size: 12px;
  color: #e74c3c;
  word-break: break-all;
}

.pagination-bar {
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
}

@keyframes fadeInUp {
  from { opacity: 0; transform: translateY(20px); }
  to { opacity: 1; transform: translateY(0); }
}

.el-table :deep(.el-table__header th .cell) {
  color: #303133;
}
</style>
