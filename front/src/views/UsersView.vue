<script setup>
import { onMounted, reactive, ref } from 'vue';
import axios from 'axios';
import { ElMessage, ElMessageBox } from 'element-plus';

const loading = ref(false);
const users = ref([]);
const combos = ref([]);
const usageLoading = ref(false);
const usageDialogVisible = ref(false);
const usageData = ref(null);

const dialogVisible = ref(false);
const isEdit = ref(false);
const formRef = ref();
const form = reactive({
  username: '',
  api_key: '',
  quota: -1,
  expire_at: '',
  is_admin: false,
  allowed_combos: [],
});

const rules = {};

// 筛选条件
const filterForm = reactive({
  username: '',
  is_admin: '',
});

const loadUsers = async () => {
  loading.value = true;
  try {
    const params = {};
    if (filterForm.username) params.username = filterForm.username;
    if (filterForm.is_admin !== '') params.is_admin = filterForm.is_admin;
    const { data } = await axios.get('/api/users', { params });
    users.value = data || [];
  } catch (_) {
    ElMessage.error('加载用户列表失败');
  } finally {
    loading.value = false;
  }
};

const loadCombos = async () => {
  try {
    const { data } = await axios.get('/api/combos');
    combos.value = (data || []).filter(c => c.enabled);
  } catch (_) {
    // 静默失败
  }
};

const openCreate = () => {
  isEdit.value = false;
  Object.assign(form, {
    username: '',
    api_key: '',
    quota: -1,
    expire_at: '',
    is_admin: false,
    allowed_combos: [],
  });
  dialogVisible.value = true;
};

const generateUsername = () => {
  const chars = 'abcdefghijklmnopqrstuvwxyz0123456789';
  let username = '';
  for (let i = 0; i < 8; i++) {
    username += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  form.username = 'user_' + username;
};

const openEdit = (row) => {
  isEdit.value = true;
  const allowedCombos = row.allowed_combos
    ? row.allowed_combos.split(',').map(s => s.trim()).filter(Boolean)
    : [];
  Object.assign(form, {
    username: row.username,
    api_key: row.api_key,
    quota: row.quota,
    expire_at: row.expire_at ? row.expire_at.slice(0, 19) : '',
    is_admin: !!row.is_admin,
    allowed_combos: allowedCombos,
  });
  dialogVisible.value = true;
};

const submit = () => {
  formRef.value.validate(async (valid) => {
    if (!valid) return;
    const payload = {
      quota: Number(form.quota),
      is_admin: !!form.is_admin,
      allowed_combos: Array.isArray(form.allowed_combos) ? form.allowed_combos.join(',') : '',
    };
    // 只有当 expire_at 有值时才添加到 payload
    if (form.expire_at && typeof form.expire_at === 'string' && form.expire_at.trim() !== '') {
      payload.expire_at = new Date(form.expire_at).toISOString();
    }

    try {
      if (isEdit.value) {
        await axios.put(`/api/users/${encodeURIComponent(form.username)}`, payload);
        ElMessage.success('更新用户成功');
      } else {
        await axios.post('/api/users', {
          username: form.username,
          ...payload,
        });
        ElMessage.success('创建用户成功（API Key 已自动生成）');
      }
      dialogVisible.value = false;
      await loadUsers();
    } catch (err) {
      console.error('保存用户失败:', err.response?.data || err.message);
      ElMessage.error(`保存用户失败: ${err.response?.data?.error || err.message}`);
    }
  });
};

const viewUsage = async (row) => {
  usageLoading.value = true;
  usageDialogVisible.value = true;
  try {
    const { data } = await axios.get(`/api/users/${encodeURIComponent(row.username)}/usage`);
    usageData.value = data;
  } catch (_) {
    usageData.value = null;
    ElMessage.error('加载使用情况失败');
  } finally {
    usageLoading.value = false;
  }
};

const deleteUser = (row) => {
  ElMessageBox.confirm(
    `确定要删除用户 "${row.username}" 吗？此操作不可撤销。`,
    '删除用户',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    }
  )
    .then(async () => {
      try {
        await axios.delete(`/api/users/${encodeURIComponent(row.username)}`);
        ElMessage.success('用户已删除');
        await loadUsers();
      } catch (_) {
        ElMessage.error('删除用户失败');
      }
    })
    .catch(() => {});
};

const resetFilter = () => {
  filterForm.username = '';
  filterForm.is_admin = '';
  loadUsers();
};

const copyToClipboard = (text) => {
  navigator.clipboard.writeText(text).then(() => {
    ElMessage.success('已复制到剪贴板');
  }).catch(() => {
    ElMessage.error('复制失败');
  });
};

// 展示 allowed_combos 的标签
const getAllowedComboLabels = (row) => {
  if (!row.allowed_combos) return [];
  return row.allowed_combos.split(',').map(s => s.trim()).filter(Boolean);
};

onMounted(() => {
  loadUsers();
  loadCombos();
});
</script>

<template>
  <div class="p-6">
    <div class="flex items-center justify-between mb-4">
      <h2 class="text-xl font-semibold">用户管理</h2>
      <el-button type="primary" @click="openCreate">新增用户</el-button>
    </div>

    <!-- 筛选表单 -->
    <div class="filter-panel mb-4">
      <el-form :model="filterForm" layout="inline" class="filter-form">
        <el-form-item label="用户名">
          <el-input
            v-model="filterForm.username"
            placeholder="搜索用户名"
            clearable
            @input="loadUsers"
          />
        </el-form-item>
        <el-form-item label="管理员">
          <el-select
            v-model="filterForm.is_admin"
            placeholder="全部"
            clearable
            @change="loadUsers"
          >
            <el-option label="是" value="true" />
            <el-option label="否" value="false" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button @click="resetFilter">重置</el-button>
        </el-form-item>
      </el-form>
    </div>

    <el-table v-loading="loading" :data="users" border style="width: 100%">
      <el-table-column prop="username" label="用户名" width="160" />
      <el-table-column prop="api_key" label="API Key" width="260">
        <template #default="{ row }">
          <div class="api-key-cell">
            <span class="api-key-text">{{ row.api_key }}</span>
            <el-button link type="primary" size="small" @click="copyToClipboard(row.api_key)">
              复制
            </el-button>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="quota" label="额度" width="100">
        <template #default="{ row }">
          {{ row.quota < 0 ? '无限' : row.quota }}
        </template>
      </el-table-column>
      <el-table-column prop="expire_at" label="过期时间" width="180">
        <template #default="{ row }">
          {{ row.expire_at || '不过期' }}
        </template>
      </el-table-column>
      <el-table-column prop="is_admin" label="管理员" width="90">
        <template #default="{ row }">
          <el-tag :type="row.is_admin ? 'danger' : 'info'">
            {{ row.is_admin ? '是' : '否' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="可用 Combo" min-width="180">
        <template #default="{ row }">
          <span v-if="!getAllowedComboLabels(row).length" class="text-gray-400 text-xs">不限制</span>
          <div v-else class="combo-tags">
            <el-tag
              v-for="id in getAllowedComboLabels(row)"
              :key="id"
              size="small"
              type="success"
              class="mr-1 mb-1"
            >
              {{ id }}
            </el-tag>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="total_tokens" label="总 Token" width="120" />
      <el-table-column label="操作" width="240" fixed="right">
        <template #default="{ row }">
          <el-button size="small" @click="openEdit(row)">编辑</el-button>
          <el-button size="small" type="primary" @click="viewUsage(row)">使用情况</el-button>
          <el-button size="small" type="danger" @click="deleteUser(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 新增/编辑用户弹窗 -->
    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑用户' : '新增用户'" width="580px">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="110px">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" :disabled="true" />
          <span v-if="!isEdit" class="form-hint-inline">系统自动生成</span>
        </el-form-item>
        <el-form-item v-if="!isEdit">
          <el-button @click="generateUsername">生成用户名</el-button>
        </el-form-item>
        <el-form-item v-if="isEdit" label="API Key" prop="api_key">
          <el-input v-model="form.api_key" show-password disabled />
        </el-form-item>
        <el-alert
          v-else
          title="创建用户时用户名和 API Key 由系统自动生成"
          type="info"
          :closable="false"
          show-icon
          class="mb-3"
        />
        <el-form-item label="额度">
          <el-input-number v-model="form.quota" :min="-1" :step="1000" />
          <span class="form-hint-inline">-1 表示无限</span>
        </el-form-item>
        <el-form-item label="过期时间">
          <el-date-picker
            v-model="form.expire_at"
            type="datetime"
            value-format="YYYY-MM-DDTHH:mm:ss"
            placeholder="可选，不填表示不过期"
            style="width: 100%"
            clearable
          />
        </el-form-item>
        <el-form-item label="管理员">
          <el-switch v-model="form.is_admin" />
        </el-form-item>
        <el-form-item label="可用 Combo">
          <el-select
            v-model="form.allowed_combos"
            multiple
            clearable
            placeholder="不选表示不限制，可使用任意模型"
            style="width: 100%"
          >
            <el-option
              v-for="combo in combos"
              :key="combo.id"
              :label="combo.name ? `${combo.name} (${combo.id})` : combo.id"
              :value="combo.id"
            />
          </el-select>
          <div class="form-hint-block">不选表示不限制，可使用任意 Combo 模型</div>
        </el-form-item>
      </el-form>

      <template #footer>
        <span class="dialog-footer">
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" @click="submit">确定</el-button>
        </span>
      </template>
    </el-dialog>

    <!-- 使用情况弹窗 -->
    <el-dialog v-model="usageDialogVisible" title="用户使用情况" width="520px">
      <el-skeleton v-if="usageLoading" :rows="6" animated />
      <div v-else-if="usageData" class="usage-panel">
        <div class="usage-row"><span>用户名</span><strong>{{ usageData.username }}</strong></div>
        <div class="usage-row"><span>输入 Token</span><strong>{{ usageData.input_tokens }}</strong></div>
        <div class="usage-row"><span>输出 Token</span><strong>{{ usageData.output_tokens }}</strong></div>
        <div class="usage-row"><span>总 Token</span><strong>{{ usageData.total_tokens }}</strong></div>
        <div class="usage-row"><span>额度</span><strong>{{ usageData.unlimited ? '无限' : usageData.quota }}</strong></div>
        <div class="usage-row"><span>剩余额度</span><strong>{{ usageData.unlimited ? '无限' : usageData.remaining }}</strong></div>
        <div class="usage-row"><span>过期时间</span><strong>{{ usageData.expire_at || '不过期' }}</strong></div>
      </div>
    </el-dialog>
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

.form-hint-inline {
  font-size: 12px;
  color: #6b7280;
  margin-left: 12px;
}

.form-hint-block {
  font-size: 12px;
  color: #6b7280;
  margin-top: 4px;
}

.api-key-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.api-key-text {
  font-family: 'Courier New', monospace;
  font-size: 12px;
  word-break: break-all;
  flex: 1;
}

.combo-tags {
  display: flex;
  flex-wrap: wrap;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

.usage-panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.usage-row {
  display: flex;
  justify-content: space-between;
  background: rgba(102, 126, 234, 0.06);
  padding: 10px 12px;
  border-radius: 8px;
}

@keyframes fadeInUp {
  from { opacity: 0; transform: translateY(20px); }
  to { opacity: 1; transform: translateY(0); }
}

.el-table :deep(.el-table__header th .cell) {
  color: #303133;
}
</style>
