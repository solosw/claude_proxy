<script setup>
import { computed, onMounted, reactive, ref } from 'vue';
import axios from 'axios';
import { ElMessage, ElMessageBox } from 'element-plus';

const loading = ref(false);
const models = ref([]);
const operators = ref([]);

const dialogVisible = ref(false);
const isEdit = ref(false);
const form = reactive({
  id: '',
  name: '',
  provider: '',
  interface_type: '',
  upstream_id: '',
  api_key: '',
  base_url: '',
  description: '',
  enabled: true,
  forward_metadata: false,
  forward_thinking: false,
  max_qps: 0,
  operator_id: '',
});

const formRules = {
  id: [{ required: true, message: '请输入模型 ID', trigger: 'blur' }],
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  provider: [{ required: true, message: '请输入服务商标识（如 openai / anthropic）', trigger: 'blur' }],
  upstream_id: [{ required: true, message: '请输入上游模型名', trigger: 'blur' }],
};

const formRef = ref();

const loadModels = async () => {
  loading.value = true;
  try {
    const { data } = await axios.get('/api/models');
    models.value = data || [];
  } catch (e) {
    ElMessage.error('加载模型列表失败');
  } finally {
    loading.value = false;
  }
};

const loadOperators = async () => {
  try {
    const { data } = await axios.get('/api/operators');
    operators.value = data || [];
  } catch (_) {
    operators.value = [];
  }
};

const openCreate = () => {
  isEdit.value = false;
  Object.assign(form, {
    id: '',
    name: '',
    provider: '',
    interface_type: 'openai',
    upstream_id: '',
    api_key: '',
    base_url: '',
    description: '',
    enabled: true,
    forward_metadata: false,
    forward_thinking: false,
    max_qps: 0,
    operator_id: '',
  });
  dialogVisible.value = true;
};

const openEdit = (row) => {
  isEdit.value = true;
  Object.assign(form, row);
  dialogVisible.value = true;
};

const submitForm = () => {
  formRef.value.validate(async (valid) => {
    if (!valid) return;
    try {
      if (isEdit.value) {
        await axios.put(`/api/models/${encodeURIComponent(form.id)}`, form);
        ElMessage.success('更新成功');
      } else {
        await axios.post('/api/models', form);
        ElMessage.success('创建成功');
      }
      dialogVisible.value = false;
      await loadModels();
    } catch (e) {
      ElMessage.error('保存失败');
    }
  });
};

const removeModel = (row) => {
  ElMessageBox.confirm(
    `确定要删除模型 ${row.id} 吗？`,
    '提示',
    { type: 'warning' },
  ).then(async () => {
    try {
      await axios.delete(`/api/models/${encodeURIComponent(row.id)}`);
      ElMessage.success('删除成功');
      await loadModels();
    } catch (e) {
      ElMessage.error('删除失败');
    }
  }).catch(() => {});
};

// --------- Chat Test ----------
const chatVisible = ref(false);
const chatStreaming = ref(true);
const chatSending = ref(false);
const selectedModel = ref(null);
const chatInput = ref('');
const chatMessages = ref([]);

const selectedInterface = computed(() => selectedModel.value?.interface_type || 'openai');

const openChat = (row) => {
  selectedModel.value = row;
  chatMessages.value = [
    {
      role: 'assistant',
      content: `正在测试模型：${row.name || row.id}\n接口类型：${row.interface_type || '(未设置)'}\n上游模型：${row.upstream_id || '(未设置)'}\n`,
    },
  ];
  chatInput.value = '';
  chatVisible.value = true;
};

const appendAssistantDelta = (delta) => {
  if (!delta) return;
  const msgs = chatMessages.value;
  const last = msgs[msgs.length - 1];
  if (!last || last.role !== 'assistant') {
    msgs.push({ role: 'assistant', content: String(delta) });
    return;
  }
  last.content += String(delta);
};

const parseOpenAISSEData = (dataLine) => {
  if (!dataLine) return null;
  if (dataLine === '[DONE]') return { done: true };
  try {
    const obj = JSON.parse(dataLine);
    const delta = obj?.choices?.[0]?.delta?.content;
    if (typeof delta === 'string' && delta.length) return { delta };
  } catch (_) {
    // ignore
  }
  return null;
};

const parseAnthropicSSEData = (dataLine) => {
  if (!dataLine) return null;
  try {
    const obj = JSON.parse(dataLine);
    // 常见：content_block_delta -> delta.text
    if (obj?.type === 'content_block_delta' && obj?.delta?.text) {
      return { delta: obj.delta.text };
    }
    // 兜底：message_delta 里可能有 stop_reason 等，不拼接
  } catch (_) {
    // ignore
  }
  return null;
};

const streamChat = async (payload) => {
  const token = localStorage.getItem('token') || '';
  const resp = await fetch('/back/api/chat/test', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      token,
    },
    body: JSON.stringify(payload),
  });
  if (!resp.ok || !resp.body) {
    const text = await resp.text();
    throw new Error(text || `HTTP ${resp.status}`);
  }

  const reader = resp.body.getReader();
  const decoder = new TextDecoder('utf-8');
  let buffer = '';

  // 预置一个 assistant 消息用于拼接 delta
  chatMessages.value.push({ role: 'assistant', content: '' });

  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    // 按行处理 SSE
    const lines = buffer.split(/\r?\n/);
    buffer = lines.pop() || '';

    for (const line of lines) {
      if (!line.startsWith('data:')) continue;
      const dataLine = line.slice(5).trim();

      let parsed = null;
      if (selectedInterface.value === 'anthropic') {
        parsed = parseAnthropicSSEData(dataLine);
      } else {
        parsed = parseOpenAISSEData(dataLine);
      }

      if (parsed?.done) return;
      if (parsed?.delta) appendAssistantDelta(parsed.delta);
    }
  }
};

const sendChat = async () => {
  if (!selectedModel.value) return;
  if (!chatInput.value.trim()) return;
  if (chatSending.value) return;

  chatSending.value = true;
  const userText = chatInput.value.trim();
  chatInput.value = '';
  chatMessages.value.push({ role: 'user', content: userText });

  const payload = {
    model_id: selectedModel.value.id,
    stream: !!chatStreaming.value,
    messages: chatMessages.value
      .filter(m => m.role === 'user' || m.role === 'assistant')
      .map(m => ({ role: m.role, content: m.content })),
  };

  try {
    if (chatStreaming.value) {
      await streamChat(payload);
      return;
    }

    const { data } = await axios.post('/api/chat/test', payload);

    if (selectedInterface.value === 'anthropic') {
      const text = (data?.content || [])
        .filter(b => b?.type === 'text')
        .map(b => b?.text || '')
        .join('');
      chatMessages.value.push({ role: 'assistant', content: text || JSON.stringify(data) });
    } else {
      const text = data?.choices?.[0]?.message?.content;
      chatMessages.value.push({ role: 'assistant', content: text || JSON.stringify(data) });
    }
  } catch (e) {
    ElMessage.error('发送失败');
    chatMessages.value.push({ role: 'assistant', content: `请求失败：${e?.message || e}` });
  } finally {
    chatSending.value = false;
  }
};

onMounted(() => {
  loadModels();
  loadOperators();
});
</script>

<template>
  <div class="p-6">
    <div class="flex items-center justify-between mb-4">
      <h2 class="text-xl font-semibold">模型管理</h2>
      <el-button type="primary" @click="openCreate">
        新建模型
      </el-button>
    </div>

    <el-table
      v-loading="loading"
      :data="models"
      border
      style="width: 100%"
    >
      <el-table-column prop="id" label="ID" width="220" />
      <el-table-column prop="name" label="名称" width="180" />
      <el-table-column prop="provider" label="服务商" width="140" />
      <el-table-column prop="interface_type" label="接口类型" width="160" />
      <el-table-column prop="upstream_id" label="上游模型名" width="220" />
      <el-table-column prop="base_url" label="Base URL" width="260" />
      <el-table-column prop="api_key" label="上游 API Key" width="260">
        <template #default="{ row }">
          <span v-if="row.api_key">••••••••</span>
          <span v-else class="text-gray-400">未配置</span>
        </template>
      </el-table-column>
      <el-table-column prop="enabled" label="启用" width="80">
        <template #default="{ row }">
          <el-tag :type="row.enabled ? 'success' : 'info'">
            {{ row.enabled ? '启用' : '停用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="扩展字段" width="120">
        <template #default="{ row }">
          <span v-if="row.operator_id" class="text-gray-400">运营商</span>
          <span v-else>
            <el-tag v-if="row.forward_metadata" size="small" type="info">metadata</el-tag>
            <el-tag v-if="row.forward_thinking" size="small" type="info">thinking</el-tag>
            <span v-if="!row.forward_metadata && !row.forward_thinking">—</span>
          </span>
        </template>
      </el-table-column>
      <el-table-column prop="max_qps" label="QPS" width="80">
        <template #default="{ row }">
          {{ row.max_qps > 0 ? row.max_qps : '—' }}
        </template>
      </el-table-column>
      <el-table-column prop="operator_id" label="运营商" width="120">
        <template #default="{ row }">
          {{ row.operator_id || '—' }}
        </template>
      </el-table-column>
      <el-table-column prop="description" label="描述" />
      <el-table-column label="操作" width="260" fixed="right">
        <template #default="{ row }">
          <el-button size="small" type="primary" @click="openChat(row)">聊天测试</el-button>
          <el-button size="small" @click="openEdit(row)">编辑</el-button>
          <el-button size="small" type="danger" @click="removeModel(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? '编辑模型' : '新建模型'"
      width="560px"
    >
      <el-form
        ref="formRef"
        :model="form"
        :rules="formRules"
        label-width="100px"
      >
        <el-form-item label="ID" prop="id">
          <el-input v-model="form.id" :disabled="isEdit" />
        </el-form-item>
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="服务商" prop="provider">
          <el-input v-model="form.provider" placeholder="例如 openai / anthropic" />
        </el-form-item>
        <el-form-item label="运营商">
          <el-select
            v-model="form.operator_id"
            placeholder="无（直连 OpenAI/Anthropic）"
            clearable
            style="width: 100%;"
          >
            <el-option
              v-for="o in operators"
              :key="o.id"
              :label="o.name || o.id"
              :value="o.id"
            />
          </el-select>
          <div class="form-hint">选择后，该模型走运营商专属 API（Base URL / API Key 以运营商为准）</div>
        </el-form-item>
        <el-form-item label="接口类型">
          <el-select
            v-model="form.interface_type"
            placeholder="选择接口类型"
            style="width: 100%;"
          >
            <el-option label="OpenAI Chat Completions" value="openai" />
            <el-option label="OpenAI 兼容" value="openai_compatible" />
            <el-option label="Anthropic Messages" value="anthropic" />
          </el-select>
          <div v-if="form.operator_id" class="form-hint">归属运营商时可由运营商配置覆盖</div>
        </el-form-item>
        <el-form-item label="上游模型名" prop="upstream_id">
          <el-input v-model="form.upstream_id" placeholder="例如 gpt-4.1 / claude-3-5-sonnet-20241022" />
        </el-form-item>
        <el-form-item label="上游 API Key">
          <el-input
            v-model="form.api_key"
            type="password"
            show-password
            :placeholder="form.operator_id ? '归属运营商时使用运营商 API Key' : '可选：为该模型配置专用上游 API Key'"
          />
        </el-form-item>
        <el-form-item label="Base URL">
          <el-input
            v-model="form.base_url"
            :placeholder="form.operator_id ? '归属运营商时使用运营商 Base URL' : '可选：为该模型覆盖 Provider 的 BaseURL'"
          />
        </el-form-item>
        <el-form-item label="转发 metadata">
          <el-switch v-model="form.forward_metadata" />
          <span class="form-hint-inline">部分上游（如 ModelScope）不支持则关闭</span>
        </el-form-item>
        <el-form-item label="转发 thinking">
          <el-switch v-model="form.forward_thinking" />
          <span class="form-hint-inline">扩展思考，不支持则关闭</span>
        </el-form-item>
        <el-form-item label="最大 QPS">
          <el-input-number v-model="form.max_qps" :min="0" :max="100" :step="0.5" />
          <span class="form-hint-inline">0 表示不限制</span>
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
      </el-form>

      <template #footer>
        <span class="dialog-footer">
          <el-button @click="dialogVisible = false">取 消</el-button>
          <el-button type="primary" @click="submitForm">确 定</el-button>
        </span>
      </template>
    </el-dialog>

    <el-drawer
      v-model="chatVisible"
      size="520px"
      :with-header="true"
      title="聊天测试"
    >
      <div class="chat-toolbar">
        <div class="chat-meta">
          <div class="chat-meta-title">{{ selectedModel?.name || selectedModel?.id }}</div>
          <div class="chat-meta-sub">接口类型：{{ selectedModel?.interface_type || 'openai' }}</div>
        </div>
        <div class="chat-actions">
          <el-switch v-model="chatStreaming" active-text="SSE" inactive-text="非流式" />
        </div>
      </div>

      <div class="chat-messages">
        <div
          v-for="(m, idx) in chatMessages"
          :key="idx"
          class="chat-bubble"
          :class="m.role === 'user' ? 'from-user' : 'from-assistant'"
        >
          <div class="chat-role">{{ m.role }}</div>
          <div class="chat-content">{{ m.content }}</div>
        </div>
      </div>

      <div class="chat-input">
        <el-input
          v-model="chatInput"
          type="textarea"
          :rows="3"
          placeholder="输入一段内容测试模型..."
          @keydown.enter.exact.prevent="sendChat"
        />
        <div class="chat-send">
          <el-button type="primary" :loading="chatSending" @click="sendChat">发送</el-button>
        </div>
      </div>
    </el-drawer>
  </div>
</template>

<style scoped>
.p-6 {
  animation: fadeInUp 0.6s ease-out;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

.form-hint {
  font-size: 12px;
  color: #6b7280;
  margin-top: 8px;
  padding: 8px 12px;
  background: rgba(102, 126, 234, 0.05);
  border-left: 3px solid #667eea;
  border-radius: 4px;
}

.form-hint-inline {
  font-size: 12px;
  color: #6b7280;
  margin-left: 12px;
  font-style: italic;
}

.chat-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
  padding: 12px;
  background: linear-gradient(135deg, rgba(102, 126, 234, 0.1), rgba(240, 147, 251, 0.1));
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.2);
}

.chat-meta-title {
  font-weight: 700;
  font-size: 16px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.chat-meta-sub {
  font-size: 12px;
  color: #6b7280;
  margin-top: 4px;
}

.chat-messages {
  height: calc(100vh - 260px);
  overflow: auto;
  padding: 12px;
  background: rgba(255, 255, 255, 0.5);
  backdrop-filter: blur(10px);
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.2);
}

.chat-bubble {
  max-width: 92%;
  padding: 12px 16px;
  margin: 10px 8px;
  border-radius: 14px;
  white-space: pre-wrap;
  word-break: break-word;
  animation: fadeInUp 0.3s ease-out;
  transition: all 0.2s ease;
}

.from-user {
  margin-left: auto;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: #fff;
  box-shadow: 0 4px 12px rgba(102, 126, 234, 0.3);
}

.from-user:hover {
  transform: translateY(-2px);
  box-shadow: 0 6px 16px rgba(102, 126, 234, 0.4);
}

.from-assistant {
  margin-right: auto;
  background: rgba(255, 255, 255, 0.8);
  color: #111827;
  border: 1px solid rgba(255, 255, 255, 0.3);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.05);
}

.from-assistant:hover {
  background: rgba(255, 255, 255, 0.95);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.chat-role {
  font-size: 11px;
  opacity: 0.7;
  margin-bottom: 6px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.chat-input {
  margin-top: 16px;
}

.chat-send {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
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

