<script setup>
import { computed, onMounted, reactive, ref } from 'vue';
import axios from 'axios';
import { ElMessage, ElMessageBox } from 'element-plus';

const loading = ref(false);
const combos = ref([]);
const models = ref([]);

const dialogVisible = ref(false);
const isEdit = ref(false);
const form = reactive({
  id: '',
  name: '',
  description: '',
  enabled: true,
  items: [],
});

const formRef = ref();

const loadData = async () => {
  loading.value = true;
  try {
    const [comboResp, modelResp] = await Promise.all([
      axios.get('/api/combos'),
      axios.get('/api/models'),
    ]);
    combos.value = comboResp.data || [];
    models.value = modelResp.data || [];
  } catch (e) {
    ElMessage.error('加载数据失败');
  } finally {
    loading.value = false;
  }
};

const modelOptions = computed(() => models.value.map(m => ({
  label: `${m.name || m.id} (${m.provider})`,
  value: m.id,
})));

const openCreate = () => {
  isEdit.value = false;
  Object.assign(form, {
    id: '',
    name: '',
    description: '',
    enabled: true,
    items: [],
  });
  dialogVisible.value = true;
};

const openEdit = (row) => {
  isEdit.value = true;
  Object.assign(form, JSON.parse(JSON.stringify(row)));
  dialogVisible.value = true;
};

const addItem = () => {
  form.items.push({
    model_id: '',
    weight: 1,
    keywords: [],
  });
};

const removeItem = (index) => {
  form.items.splice(index, 1);
};

const submitForm = () => {
  // 简单校验：至少一个子模型
  if (!form.items || form.items.length === 0) {
    ElMessage.error('请至少添加一个子模型');
    return;
  }

  // 将 keywords 从逗号分隔的字符串转换为数组（若用户输入字符串形式）
  const payload = {
    ...form,
    items: form.items.map(item => ({
      model_id: item.model_id,
      weight: Number(item.weight) || 1,
      keywords: Array.isArray(item.keywords)
        ? item.keywords
        : String(item.keywords || '')
          .split(',')
          .map(s => s.trim())
          .filter(Boolean),
    })),
  };

  formRef.value?.validate?.(async (valid) => {
    if (valid === false) return;
    try {
      if (isEdit.value) {
        await axios.put(`/api/combos/${encodeURIComponent(form.id)}`, payload);
        ElMessage.success('更新成功');
      } else {
        await axios.post('/api/combos', payload);
        ElMessage.success('创建成功');
      }
      dialogVisible.value = false;
      await loadData();
    } catch (e) {
      ElMessage.error('保存失败');
    }
  });
};

const removeCombo = (row) => {
  ElMessageBox.confirm(
    `确定要删除组合模型 ${row.id} 吗？`,
    '提示',
    { type: 'warning' },
  ).then(async () => {
    try {
      await axios.delete(`/api/combos/${encodeURIComponent(row.id)}`);
      ElMessage.success('删除成功');
      await loadData();
    } catch (e) {
      ElMessage.error('删除失败');
    }
  }).catch(() => {});
};

onMounted(loadData);
</script>

<template>
  <div class="p-6">
    <div class="flex items-center justify-between mb-4">
      <h2 class="text-xl font-semibold">组合模型管理</h2>
      <el-button type="primary" @click="openCreate">
        新建组合模型
      </el-button>
    </div>

    <el-table
      v-loading="loading"
      :data="combos"
      border
      style="width: 100%"
    >
      <el-table-column prop="id" label="ID" width="220" />
      <el-table-column prop="name" label="名称" width="200" />
      <el-table-column prop="enabled" label="启用" width="100">
        <template #default="{ row }">
          <el-tag :type="row.enabled ? 'success' : 'info'">
            {{ row.enabled ? '启用' : '停用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="子模型" width="260">
        <template #default="{ row }">
          <div v-for="item in row.items" :key="item.model_id" class="text-xs text-gray-700">
            <span class="font-medium">{{ item.model_id }}</span>
            <span class="ml-1 text-gray-500">权重: {{ item.weight }}</span>
            <span v-if="item.keywords && item.keywords.length" class="ml-1 text-gray-400">
              关键词: {{ item.keywords.join(', ') }}
            </span>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="description" label="描述" />
      <el-table-column label="操作" width="180" fixed="right">
        <template #default="{ row }">
          <el-button size="small" @click="openEdit(row)">编辑</el-button>
          <el-button size="small" type="danger" @click="removeCombo(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? '编辑组合模型' : '新建组合模型'"
      width="640px"
    >
      <el-form ref="formRef" :model="form" label-width="100px">
        <el-form-item label="ID">
          <el-input v-model="form.id" :disabled="isEdit" />
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input
            v-model="form.description"
            type="textarea"
            :rows="3"
          />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>

        <el-form-item label="子模型">
          <div class="w-full">
            <div class="flex justify-end mb-2">
              <el-button size="small" @click="addItem">
                添加子模型
              </el-button>
            </div>

            <el-table
              :data="form.items"
              border
              size="small"
            >
              <el-table-column label="模型" width="260">
                <template #default="{ row }">
                  <el-select
                    v-model="row.model_id"
                    placeholder="选择模型"
                    filterable
                    style="width: 240px"
                  >
                    <el-option
                      v-for="opt in modelOptions"
                      :key="opt.value"
                      :label="opt.label"
                      :value="opt.value"
                    />
                  </el-select>
                </template>
              </el-table-column>
              <el-table-column label="权重" width="120">
                <template #default="{ row }">
                  <el-input-number
                    v-model="row.weight"
                    :min="0"
                    :max="100"
                    :step="0.1"
                  />
                </template>
              </el-table-column>
              <el-table-column label="关键词" width="220">
                <template #default="{ row }">
                  <el-input
                    v-model="row.keywords"
                    placeholder="用逗号分隔，例如: 图表, 文本"
                  />
                </template>
              </el-table-column>
              <el-table-column label="操作" width="80">
                <template #default="scope">
                  <el-button
                    size="small"
                    type="danger"
                    @click="removeItem(scope.$index)"
                  >
                    删除
                  </el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </el-form-item>
      </el-form>

      <template #footer>
        <span class="dialog-footer">
          <el-button @click="dialogVisible = false">取 消</el-button>
          <el-button type="primary" @click="submitForm">确 定</el-button>
        </span>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>

