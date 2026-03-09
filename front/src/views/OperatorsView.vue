<script setup>
import { onMounted, ref } from 'vue';
import axios from 'axios';
import { ElMessage } from 'element-plus';

const loading = ref(false);
const operators = ref([]);

const loadOperators = async () => {
  loading.value = true;
  try {
    const { data } = await axios.get('/api/operators');
    operators.value = data || [];
  } catch (e) {
    ElMessage.error('加载运营商列表失败');
  } finally {
    loading.value = false;
  }
};

onMounted(loadOperators);
</script>

<template>
  <div class="p-6">
    <div class="header-bar mb-6">
      <div>
        <h2 class="page-title"><el-icon class="mr-2"><Connection /></el-icon>运营商</h2>
        <p class="page-description">
          运营商为系统内置，不可添加或修改。选择运营商的模型会使用系统配置的转发逻辑，与直连 OpenAI / Anthropic 区分开。
        </p>
      </div>
    </div>

    <el-table
      v-loading="loading"
      :data="operators"
      border
      style="width: 100%"
    >
      <el-table-column prop="id" label="ID" width="160" />
      <el-table-column prop="name" label="名称" width="200" />
      <el-table-column prop="description" label="描述" />
      <el-table-column prop="enabled" label="启用" width="100">
        <template #default="{ row }">
          <el-tag :type="row.enabled ? 'success' : 'info'">
            {{ row.enabled ? '启用' : '停用' }}
          </el-tag>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<style scoped>
.p-6 {
  animation: fadeInUp 0.6s ease-out;
}

.header-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.page-title {
  display: flex;
  align-items: center;
  font-size: 24px;
  font-weight: 700;
  margin: 0 0 12px;
  color: #f8fafc;
}

.page-title .el-icon {
  color: #34d399;
}

.page-description {
  margin: 0;
  font-size: 14px;
  color: #94a3b8;
  line-height: 1.6;
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
