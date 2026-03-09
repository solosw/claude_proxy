<script setup>
import { ref, onMounted } from 'vue';
import axios from 'axios';
import { ElMessage, ElMessageBox } from 'element-plus';

const loading = ref(false);
const codes = ref([]);
const total = ref(0);
const currentPage = ref(1);
const pageSize = ref(10);

const dialogVisible = ref(false);
const dialogTitle = ref('创建兑换码');
const formLoading = ref(false);

const form = ref({
  code: '',
  quota: null,
  max_uses: 1,
  expire_at: null,
  description: '',
});

const rules = {
  quota: [
    { required: true, message: '额度不能为空', trigger: 'blur' },
    { type: 'number', min: 0.01, message: '额度必须大于0', trigger: 'blur' },
  ],
  max_uses: [
    { required: true, message: '最大使用次数不能为空', trigger: 'blur' },
    { type: 'number', min: 1, message: '最大使用次数必须大于等于1', trigger: 'blur' },
  ],
};

const loadCodes = async () => {
  loading.value = true;
  try {
    const { data } = await axios.get('/api/redeem-codes', {
      params: {
        page: currentPage.value,
        page_size: pageSize.value,
      },
    });
    codes.value = data.data?.codes || [];
    total.value = data.data?.total || 0;
  } catch (err) {
    ElMessage.error('加载兑换码失败');
  } finally {
    loading.value = false;
  }
};

const handlePageChange = (page) => {
  currentPage.value = page;
  loadCodes();
};

const openCreateDialog = () => {
  dialogTitle.value = '创建兑换码';
  form.value = {
    code: '',
    quota: null,
    max_uses: 1,
    expire_at: null,
    description: '',
  };
  dialogVisible.value = true;
};

const handleCreateCode = async () => {
  if (!form.value.quota || form.value.quota <= 0) {
    ElMessage.error('请输入有效的额度');
    return;
  }
  if (!form.value.max_uses || form.value.max_uses < 1) {
    ElMessage.error('请输入有效的最大使用次数');
    return;
  }

  formLoading.value = true;
  try {
    const payload = {
      code: form.value.code || undefined,
      quota: form.value.quota,
      max_uses: form.value.max_uses,
      expire_at: form.value.expire_at || null,
      description: form.value.description,
    };
    await axios.post('/api/redeem-codes', payload);
    ElMessage.success('兑换码创建成功');
    dialogVisible.value = false;
    loadCodes();
  } catch (err) {
    const msg = err.response?.data?.message || '创建失败';
    ElMessage.error(msg);
  } finally {
    formLoading.value = false;
  }
};

const handleDeleteCode = (id) => {
  ElMessageBox.confirm('确定要删除该兑换码吗？', '提示', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    type: 'warning',
  })
    .then(async () => {
      try {
        await axios.delete(`/api/redeem-codes/${id}`);
        ElMessage.success('删除成功');
        loadCodes();
      } catch (err) {
        ElMessage.error('删除失败');
      }
    })
    .catch(() => {});
};

const copyToClipboard = (text) => {
  navigator.clipboard.writeText(text).then(() => {
    ElMessage.success('已复制到剪贴板');
  }).catch(() => {
    ElMessage.error('复制失败');
  });
};

const formatDate = (date) => {
  if (!date) return '-';
  return new Date(date).toLocaleString();
};

const getCodeStatus = (code) => {
  if (code.expire_at && new Date(code.expire_at) < new Date()) {
    return '已过期';
  }
  if (code.used_count >= code.max_uses) {
    return '已用完';
  }
  return '可用';
};

const getStatusType = (code) => {
  if (code.expire_at && new Date(code.expire_at) < new Date()) {
    return 'danger';
  }
  if (code.used_count >= code.max_uses) {
    return 'warning';
  }
  return 'success';
};

onMounted(() => {
  loadCodes();
});
</script>

<template>
  <div class="p-6">
    <div class="header-bar mb-6">
      <div class="header-content">
        <div>
          <h2 class="page-title"><el-icon class="mr-2"><Grid /></el-icon>兑换码管理</h2>
          <p class="page-description">创建和管理用户兑换码，用户可以通过兑换码获得额度。</p>
        </div>
        <el-button type="primary" @click="openCreateDialog">
          + 创建兑换码
        </el-button>
      </div>
    </div>

    <el-table
      :data="codes"
      :loading="loading"
      stripe
      style="width: 100%"
      class="redeem-codes-table"
    >
      <el-table-column prop="code" label="兑换码" width="150">
        <template #default="{ row }">
          <div class="code-cell">
            <span class="code-text">{{ row.code }}</span>
            <el-button
              link
              type="primary"
              size="small"
              @click="copyToClipboard(row.code)"
            >
              复制
            </el-button>
          </div>
        </template>
      </el-table-column>

      <el-table-column prop="quota" label="额度" width="100" align="right" />

      <el-table-column label="使用情况" width="120" align="center">
        <template #default="{ row }">
          {{ row.used_count }} / {{ row.max_uses }}
        </template>
      </el-table-column>

      <el-table-column label="状态" width="100" align="center">
        <template #default="{ row }">
          <el-tag :type="getStatusType(row)">
            {{ getCodeStatus(row) }}
          </el-tag>
        </template>
      </el-table-column>

      <el-table-column prop="description" label="描述" min-width="150" />

      <el-table-column label="过期时间" width="180">
        <template #default="{ row }">
          {{ formatDate(row.expire_at) }}
        </template>
      </el-table-column>

      <el-table-column label="创建者" width="120">
        <template #default="{ row }">
          {{ row.created_by }}
        </template>
      </el-table-column>

      <el-table-column label="创建时间" width="180">
        <template #default="{ row }">
          {{ formatDate(row.created_at) }}
        </template>
      </el-table-column>

      <el-table-column label="操作" width="100" fixed="right">
        <template #default="{ row }">
          <el-button
            link
            type="danger"
            size="small"
            @click="handleDeleteCode(row.id)"
          >
            删除
          </el-button>
        </template>
      </el-table-column>
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

    <!-- 创建兑换码对话框 -->
    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="500px">
      <el-form :model="form" :rules="rules" label-width="120px">
        <el-form-item label="兑换码" prop="code">
          <el-input
            v-model="form.code"
            placeholder="留空则自动生成"
            clearable
          />
          <div class="form-tip">留空则系统自动生成随机兑换码</div>
        </el-form-item>

        <el-form-item label="额度" prop="quota" required>
          <el-input-number
            v-model="form.quota"
            :min="0.01"
            :step="1"
            placeholder="请输入额度"
          />
        </el-form-item>

        <el-form-item label="最大使用次数" prop="max_uses" required>
          <el-input-number
            v-model="form.max_uses"
            :min="1"
            :step="1"
            placeholder="请输入最大使用次数"
          />
          <div class="form-tip">1 = 单次使用，>1 = 多次使用</div>
        </el-form-item>

        <el-form-item label="过期时间" prop="expire_at">
          <el-date-picker
            v-model="form.expire_at"
            type="datetime"
            placeholder="选择过期时间"
            clearable
          />
          <div class="form-tip">留空则永不过期</div>
        </el-form-item>

        <el-form-item label="描述" prop="description">
          <el-input
            v-model="form.description"
            type="textarea"
            rows="3"
            placeholder="输入兑换码描述（可选）"
          />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="formLoading" @click="handleCreateCode">
          创建
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.p-6 {
  animation: fadeInUp 0.6s ease-out;
}

.header-bar {
  margin-bottom: 24px;
}

.header-content {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
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

.redeem-codes-table {
  margin-bottom: 16px;
}



.code-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.code-text {
  font-family: 'Courier New', monospace;
  font-size: 12px;
  font-weight: 600;
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

.form-tip {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
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
