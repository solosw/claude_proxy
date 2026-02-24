<script setup>
import { onMounted, reactive, ref } from 'vue';
import axios from 'axios';
import { ElMessage } from 'element-plus';

const loading = ref(false);
const users = ref([]);
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
});

const rules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
};

const loadUsers = async () => {
  loading.value = true;
  try {
    const { data } = await axios.get('/api/users');
    users.value = data || [];
  } catch (_) {
    ElMessage.error('加载用户列表失败');
  } finally {
    loading.value = false;
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
  });
  dialogVisible.value = true;
};

const openEdit = (row) => {
  isEdit.value = true;
  Object.assign(form, {
    username: row.username,
    api_key: row.api_key,
    quota: row.quota,
    expire_at: row.expire_at ? row.expire_at.slice(0, 19) : '',
    is_admin: !!row.is_admin,
  });
  dialogVisible.value = true;
};

const submit = () => {
  formRef.value.validate(async (valid) => {
    if (!valid) return;
    const payload = {
      quota: Number(form.quota),
      is_admin: !!form.is_admin,
    };
    if (form.expire_at) {
      payload.expire_at = new Date(form.expire_at).toISOString();
    }

    try {
      if (isEdit.value) {
        await axios.put(`/api/users/${encodeURIComponent(form.username)}`, {
          ...payload,
          api_key: form.api_key,
        });
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
    } catch (_) {
      ElMessage.error('保存用户失败');
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

onMounted(loadUsers);
</script>

<template>
  <div class="p-6">
    <div class="flex items-center justify-between mb-4">
      <h2 class="text-xl font-semibold">用户管理</h2>
      <el-button type="primary" @click="openCreate">新增用户</el-button>
    </div>

    <el-table v-loading="loading" :data="users" border style="width: 100%">
      <el-table-column prop="username" label="用户名" width="180" />
      <el-table-column prop="api_key" label="API Key" width="240">
        <template #default="{ row }">
          <span v-if="row.api_key">••••••••</span>
          <span v-else class="text-gray-400">未设置</span>
        </template>
      </el-table-column>
      <el-table-column prop="quota" label="额度" width="120">
        <template #default="{ row }">
          {{ row.quota < 0 ? '无限' : row.quota }}
        </template>
      </el-table-column>
      <el-table-column prop="expire_at" label="过期时间" width="220">
        <template #default="{ row }">
          {{ row.expire_at || '不过期' }}
        </template>
      </el-table-column>
      <el-table-column prop="is_admin" label="管理员" width="120">
        <template #default="{ row }">
          <el-tag :type="row.is_admin ? 'danger' : 'info'">
            {{ row.is_admin ? '是' : '否' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="total_tokens" label="总 Token" width="140" />
      <el-table-column label="操作" width="220" fixed="right">
        <template #default="{ row }">
          <el-button size="small" @click="openEdit(row)">编辑</el-button>
          <el-button size="small" type="primary" @click="viewUsage(row)">使用情况</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑用户' : '新增用户'" width="560px">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" :disabled="isEdit" />
        </el-form-item>
        <el-form-item v-if="isEdit" label="API Key" prop="api_key">
          <el-input v-model="form.api_key" show-password />
        </el-form-item>
        <el-alert
          v-else
          title="创建用户时 API Key 由系统自动生成（32位字母数字）"
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
      </el-form>

      <template #footer>
        <span class="dialog-footer">
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" @click="submit">确定</el-button>
        </span>
      </template>
    </el-dialog>

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

.form-hint-inline {
  font-size: 12px;
  color: #6b7280;
  margin-left: 12px;
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
