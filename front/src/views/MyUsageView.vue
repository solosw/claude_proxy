<script setup>
import { computed, onMounted, ref } from 'vue';
import axios from 'axios';
import { ElMessage, ElMessageBox } from 'element-plus';
import { useRouter } from 'vue-router';
import {
  User,
  Wallet,
  Odometer,
  DataLine,
  Clock,
  Key,
  Present,
  Document,
  Collection,
  CopyDocument,
  SwitchButton
} from '@element-plus/icons-vue';

const router = useRouter();
const loading = ref(false);
const usage = ref(null);
const usageLogs = ref([]);
const logsLoading = ref(false);
const currentPage = ref(1);
const pageSize = ref(10);
const total = ref(0);
const activeGuideTab = ref('claude-code');

// 兑换码相关
const redeemCode = ref('');
const redeemLoading = ref(false);

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

const allowedComboNames = computed(() => {
  if (!usage.value?.allowed_combos) return [];
  return usage.value.allowed_combos.split(',').map(s => s.trim()).filter(Boolean);
});

const quotaPercentage = computed(() => {
  if (!usage.value || usage.value.unlimited) return 100;
  const quota = usage.value.quota || 0;
  const remaining = usage.value.remaining || 0;
  if (quota <= 0) return 0;
  return Math.max(0, Math.min(100, (remaining / quota) * 100));
});

const anthropicBaseUrl = computed(() => `${window.location.origin}/back`);
const openaiBaseUrl = computed(() => `${window.location.origin}/back/v1`);

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

const handleRedeem = async () => {
  const code = redeemCode.value.trim();
  if (!code) {
    ElMessage.warning('请输入兑换码');
    return;
  }

  redeemLoading.value = true;
  try {
    await axios.post('/api/redeem', { code });
    ElMessage.success('兑换成功！额度已增加');
    redeemCode.value = '';
    await loadMyUsage();
  } catch (err) {
    const msg = err.response?.data?.message || '兑换失败';
    ElMessage.error(msg);
  } finally {
    redeemLoading.value = false;
  }
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
  <div class="main-content">
    <!-- 头部区域 -->
      <div class="header-section glass-panel">
        <div class="header-content">
          <div class="header-text">
            <h1 class="page-title">
              <span class="gradient-text">Welcome Back</span>
            </h1>
            <p class="page-description">实时查看您的额度、使用统计与 Token 消耗情况</p>
          </div>
          <el-button color="#ef4444" plain @click="handleLogout" class="logout-btn custom-btn">
            <el-icon class="mr-1"><SwitchButton /></el-icon> 退出登录
          </el-button>
        </div>
      </div>

      <el-skeleton v-if="loading" :rows="8" animated class="glass-panel p-6" />

      <div v-else-if="usage" class="dashboard-grid">
        <!-- 主要指标卡片 -->
        <div class="metrics-grid">
          <!-- 用户信息卡 -->
          <div class="metric-card user-card">
            <div class="card-icon-wrapper user-icon">
              <el-icon><User /></el-icon>
            </div>
            <div class="card-content">
              <div class="card-label">用户名</div>
              <div class="card-value">{{ usage.username }}</div>
            </div>
          </div>

          <!-- 额度卡 -->
          <div class="metric-card quota-card">
            <div class="card-icon-wrapper wallet-icon">
              <el-icon><Wallet /></el-icon>
            </div>
            <div class="card-content">
              <div class="card-label">总额度</div>
              <div class="card-value">{{ usage.unlimited ? '无限' : usage.quota }}</div>
              <div v-if="!usage.unlimited" class="quota-bar">
                <div class="quota-progress" :style="{ width: quotaPercentage + '%' }"></div>
              </div>
            </div>
          </div>

          <!-- 剩余额度卡 -->
          <div class="metric-card remaining-card">
            <div class="card-icon-wrapper odometer-icon">
              <el-icon><Odometer /></el-icon>
            </div>
            <div class="card-content">
              <div class="card-label">剩余额度</div>
              <div class="card-value">{{ usage.unlimited ? '无限' : usage.remaining }}</div>
              <div class="remaining-percent highlight-text">{{ quotaPercentage.toFixed(1) }}% 可用</div>
            </div>
          </div>

          <!-- Token 统计卡 -->
          <div class="metric-card token-card">
            <div class="card-icon-wrapper data-icon">
              <el-icon><DataLine /></el-icon>
            </div>
            <div class="card-content">
              <div class="card-label">总计 Token</div>
              <div class="card-value">{{ usage.total_tokens }}</div>
            </div>
          </div>

          <!-- 计费模式卡 -->
          <div class="metric-card billing-card">
            <div class="card-icon-wrapper" style="color: #f472b6; background: rgba(244, 114, 182, 0.15); border-color: rgba(244, 114, 182, 0.3);">
              <el-icon><Wallet /></el-icon>
            </div>
            <div class="card-content">
              <div class="card-label">计费模式</div>
              <div class="card-value">{{ usage.billing_mode === 'request' ? '按次数' : '按Token' }}</div>
              <div v-if="usage.billing_mode === 'request'" class="remaining-percent highlight-text">{{ usage.request_price }} 额度/次</div>
            </div>
          </div>

          <!-- 总请求次数卡 -->
          <div class="metric-card request-card">
            <div class="card-icon-wrapper" style="color: #34d399; background: rgba(52, 211, 153, 0.15); border-color: rgba(52, 211, 153, 0.3);">
              <el-icon><Odometer /></el-icon>
            </div>
            <div class="card-content">
              <div class="card-label">总请求次数</div>
              <div class="card-value">{{ usage.total_requests || 0 }}</div>
            </div>
          </div>

          <!-- 过期时间卡 -->
          <div class="metric-card expire-card">
            <div class="card-icon-wrapper clock-icon">
              <el-icon><Clock /></el-icon>
            </div>
            <div class="card-content">
              <div class="card-label">过期时间</div>
              <div class="card-value">{{ usage.expire_at ? new Date(usage.expire_at).toLocaleDateString() : '永不过期' }}</div>
            </div>
          </div>

          <!-- API Key 卡 -->
          <div class="metric-card api-card">
            <div class="card-icon-wrapper key-icon">
              <el-icon><Key /></el-icon>
            </div>
            <div class="card-content">
              <div class="card-label">API Key</div>
              <div class="api-key-display">
                <span class="api-key-text">{{ usage.api_key }}</span>
                <el-button color="#4f7cff" plain size="small" @click="copyToClipboard(usage.api_key)" class="copy-btn custom-btn">
                  <el-icon><CopyDocument /></el-icon>
                </el-button>
              </div>
            </div>
          </div>
        </div>

        <div class="middle-section">
          <!-- 可用模型 -->
          <div class="models-section glass-panel">
            <div class="section-header">
              <div class="section-icon"><el-icon><DataLine /></el-icon></div>
              <h3 class="section-title">可用模型</h3>
            </div>
            <div v-if="allowedComboNames.length" class="models-container">
              <el-tag v-for="name in allowedComboNames" :key="name" effect="dark" class="model-tag premium-tag">
                {{ name }}
              </el-tag>
            </div>
            <div v-else class="models-empty">
              <span>🚀 全场畅行（所有模型均可使用）</span>
            </div>
          </div>

          <!-- 兑换码区域 -->
          <div class="redeem-section glass-panel highlight-panel">
            <div class="section-header">
              <div class="section-icon present-icon"><el-icon><Present /></el-icon></div>
              <div>
                <h3 class="section-title m-0">兑换额度</h3>
                <p class="redeem-subtitle">输入专属兑换码，立即提升您的使用配额</p>
              </div>
            </div>
            <div class="redeem-input-wrapper">
              <el-input
                v-model="redeemCode"
                placeholder="在此输入或粘贴兑换码"
                clearable
                @keyup.enter="handleRedeem"
                class="redeem-input custom-input"
              >
                <template #prefix>
                  <el-icon><Key /></el-icon>
                </template>
              </el-input>
              <el-button
                color="#667eea"
                :loading="redeemLoading"
                @click="handleRedeem"
                class="redeem-btn custom-btn glow-btn"
              >
                立即兑换
              </el-button>
            </div>
          </div>
        </div>

        <!-- 使用日志 -->
        <div class="logs-section glass-panel">
          <div class="section-header">
            <div class="section-icon log-icon"><el-icon><Document /></el-icon></div>
            <h3 class="section-title">使用日志</h3>
          </div>
          <el-table
            :data="usageLogs"
            :loading="logsLoading"
            class="usage-logs-table premium-table"
          >
            <el-table-column prop="model_id" label="模型名" min-width="160">
              <template #default="{ row }">
                <span class="model-name-badge">{{ row.model_id }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="provider" label="供应商" min-width="120">
              <template #default="{ row }">
                <span class="provider-badge">{{ row.provider }}</span>
              </template>
            </el-table-column>
            <el-table-column label="计费模式" width="100">
              <template #default="{ row }">
                <el-tag :type="row.billing_mode === 'request' ? 'warning' : 'success'" size="small">
                  {{ row.billing_mode === 'request' ? '按次' : '按Token' }}
                </el-tag>
              </template>
            </el-table-column>
            <!-- 按次计费显示请求次数 -->
            <el-table-column v-if="usage?.billing_mode === 'request'" label="请求次数" align="right" min-width="100">
              <template #default="{ row }">
                <span class="token-val">{{ row.request_count || 0 }}</span>
              </template>
            </el-table-column>
            <el-table-column v-if="usage?.billing_mode === 'request'" label="单价" align="right" min-width="100">
              <template #default="{ row }">
                <span class="token-val">{{ row.request_price || 0 }} 额度</span>
              </template>
            </el-table-column>
            <!-- 按Token计费显示token -->
            <el-table-column v-if="usage?.billing_mode !== 'request'" label="输入 Token" align="right" min-width="120">
              <template #default="{ row }">
                <span class="token-val input-token">{{ row.input_tokens || 0 }}</span>
              </template>
            </el-table-column>
            <el-table-column v-if="usage?.billing_mode !== 'request'" label="输出 Token" align="right" min-width="120">
              <template #default="{ row }">
                <span class="token-val output-token">{{ row.output_tokens || 0 }}</span>
              </template>
            </el-table-column>
            <el-table-column v-if="usage?.billing_mode !== 'request'" label="总 Token" align="right" min-width="120">
              <template #default="{ row }">
                <span class="token-val total-token">{{ (row.input_tokens || 0) + (row.output_tokens || 0) }}</span>
              </template>
            </el-table-column>
            <el-table-column label="总费用" align="right" min-width="140">
              <template #default="{ row }">
                <span class="cost-val"> {{ row.total_cost ? row.total_cost.toFixed(6) : '0.000000' }}</span>
              </template>
            </el-table-column>
            <el-table-column label="时间" align="right" min-width="180">
              <template #default="{ row }">
                <span class="time-val">{{ row.created_at ? new Date(row.created_at).toLocaleString() : '-' }}</span>
              </template>
            </el-table-column>
          </el-table>

          <div class="pagination-container">
            <el-pagination
              v-model:current-page="currentPage"
              :page-size="pageSize"
              :total="total"
              layout="total, prev, pager, next"
              @current-change="handlePageChange"
              background
              class="custom-pagination"
            />
          </div>
        </div>

        <!-- 使用说明 -->
        <div class="guide-section glass-panel">
          <div class="section-header">
            <div class="section-icon guide-icon"><el-icon><Collection /></el-icon></div>
            <h3 class="section-title">接入指南</h3>
          </div>
          <el-tabs v-model="activeGuideTab" class="guide-tabs custom-tabs">
            <el-tab-pane label="Claude Code" name="claude-code">
              <div class="guide-block">
                <p class="guide-text">适用于日常开发与代码协作。将当前账号 API Key 配置到 Anthropic 环境变量后即可使用。</p>
                <div class="guide-step">1. 终端配置环境变量</div>
                <div class="guide-code-line">
                  <code>export ANTHROPIC_BASE_URL="{{ anthropicBaseUrl }}"</code>
                  <el-button color="rgba(255,255,255,0.1)" @click="copyToClipboard(`export ANTHROPIC_BASE_URL=&quot;${anthropicBaseUrl}&quot;`)" class="copy-btn-small"><el-icon><CopyDocument /></el-icon></el-button>
                </div>
                <div class="guide-code-line">
                  <code>export ANTHROPIC_API_KEY="{{ usage.api_key }}"</code>
                  <el-button color="rgba(255,255,255,0.1)" @click="copyToClipboard(`export ANTHROPIC_API_KEY=&quot;${usage.api_key}&quot;`)" class="copy-btn-small"><el-icon><CopyDocument /></el-icon></el-button>
                </div>
                <div class="guide-step">2. 启动终端项目</div>
                <div class="guide-code-line">
                  <code>claude</code>
                  <el-button color="rgba(255,255,255,0.1)" @click="copyToClipboard('claude')" class="copy-btn-small"><el-icon><CopyDocument /></el-icon></el-button>
                </div>
              </div>
            </el-tab-pane>

            <el-tab-pane label="Codex" name="codex">
              <div class="guide-block">
                <p class="guide-text">Codex 客户端通常可通过 OpenAI Response 接入本项目网关。</p>
                <div class="guide-step">配置 OpenAI 兼容地址与 Key</div>
                <div class="guide-code-line">
                  <code>OPENAI_BASE_URL="{{ openaiBaseUrl }}"</code>
                  <el-button color="rgba(255,255,255,0.1)" @click="copyToClipboard(openaiBaseUrl)" class="copy-btn-small"><el-icon><CopyDocument /></el-icon></el-button>
                </div>
                <div class="guide-code-line">
                  <code>OPENAI_API_KEY="{{ usage.api_key }}"</code>
                  <el-button color="rgba(255,255,255,0.1)" @click="copyToClipboard(usage.api_key)" class="copy-btn-small"><el-icon><CopyDocument /></el-icon></el-button>
                </div>
              </div>
            </el-tab-pane>

            <el-tab-pane label="OpenCode" name="opencode">
              <div class="guide-block">
                <p class="guide-text">OpenCode 同样可按 OpenAI chat 方式配置</p>
                <div class="guide-code-line">
                  <code>OPENAI_BASE_URL="{{ openaiBaseUrl }}"</code>
                  <el-button color="rgba(255,255,255,0.1)" @click="copyToClipboard(openaiBaseUrl)" class="copy-btn-small"><el-icon><CopyDocument /></el-icon></el-button>
                </div>
                <div class="guide-code-line">
                  <code>OPENAI_API_KEY="{{ usage.api_key }}"</code>
                  <el-button color="rgba(255,255,255,0.1)" @click="copyToClipboard(usage.api_key)" class="copy-btn-small"><el-icon><CopyDocument /></el-icon></el-button>
                </div>
              </div>
            </el-tab-pane>

            <el-tab-pane label="第三方客户端" name="project-config">
              <div class="guide-block">
                <p class="guide-text">其他支持任何自定义模型 OpenAI chat 接口的开发工具（如 Cursor, Continue.dev 等）。</p>
                <div class="guide-code-line">
                  <code>BASE_URL = {{ openaiBaseUrl }}</code>
                  <el-button color="rgba(255,255,255,0.1)" @click="copyToClipboard(openaiBaseUrl)" class="copy-btn-small"><el-icon><CopyDocument /></el-icon></el-button>
                </div>
                <div class="guide-code-line">
                  <code>API_KEY = {{ usage.api_key }}</code>
                  <el-button color="rgba(255,255,255,0.1)" @click="copyToClipboard(usage.api_key)" class="copy-btn-small"><el-icon><CopyDocument /></el-icon></el-button>
                </div>
              </div>
            </el-tab-pane>
          </el-tabs>
        </div>
    </div>
  </div>
</template>

<style scoped>
:root {
  --primary-color: #667eea;
  --primary-dark: #5568d3;
  --accent-color: #4f7cff;
  --success-color: #10b981;
  --warning-color: #f59e0b;
  --danger-color: #ef4444;
}

* {
  box-sizing: border-box;
}

.mr-1 {
  margin-right: 4px;
}
.m-0 {
  margin: 0 !important;
}
.p-6 {
  padding: 24px;
}

.main-content {
  position: relative;
  z-index: 10;
  max-width: 1280px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 24px;
  animation: fadeIn 0.6s ease-out;
}

/* 玻璃拟态面板基础样式 */
.glass-panel {
  background: rgba(15, 23, 42, 0.6);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 20px;
  box-shadow: 0 10px 30px -10px rgba(0, 0, 0, 0.5);
  transition: transform 0.3s ease, box-shadow 0.3s ease, border-color 0.3s ease;
}

.glass-panel:hover {
  border-color: rgba(255, 255, 255, 0.15);
  box-shadow: 0 10px 40px -10px rgba(102, 126, 234, 0.2);
}

.custom-btn {
  border-radius: 10px;
  font-weight: 500;
  backdrop-filter: blur(4px);
  transition: all 0.3s ease;
  border: 1px solid rgba(255,255,255,0.1);
}
.custom-btn:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0,0,0,0.2);
}

/* Header */
.header-section {
  padding: 32px;
  background: linear-gradient(135deg, rgba(30,35,50,0.7) 0%, rgba(20,25,40,0.7) 100%);
  border-left: 4px solid #667eea;
}
.header-content {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 24px;
}
.page-title {
  font-size: 36px;
  font-weight: 900;
  margin: 0 0 8px;
  letter-spacing: -0.5px;
}
.gradient-text {
  background: linear-gradient(135deg, #fff 0%, #a5b4fc 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}
.page-description {
  margin: 0;
  font-size: 15px;
  color: #94a3b8;
  line-height: 1.6;
}

/* 指标统计布局 */
.dashboard-grid {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.metrics-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 20px;
}

/* 单个指标卡片 */
.metric-card {
  background: rgba(20, 25, 40, 0.6);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: 16px;
  padding: 24px;
  display: flex;
  align-items: center;
  gap: 20px;
  position: relative;
  overflow: hidden;
  transition: all 0.4s cubic-bezier(0.175, 0.885, 0.32, 1.275);
  box-shadow: inset 0 0 0 1px rgba(255,255,255,0.02);
}

.metric-card::before {
  content: '';
  position: absolute;
  top: -50%; left: -50%;
  width: 200%; height: 200%;
  background: radial-gradient(circle at center, rgba(102,126,234,0.1) 0%, transparent 50%);
  opacity: 0;
  transition: opacity 0.5s ease;
  pointer-events: none;
}

.metric-card:hover {
  transform: translateY(-6px) scale(1.02);
  border-color: rgba(102, 126, 234, 0.4);
  box-shadow: 0 15px 35px -5px rgba(10, 15, 30, 0.6), 0 0 20px -5px rgba(102, 126, 234, 0.3);
}

.metric-card:hover::before {
  opacity: 1;
}

.card-icon-wrapper {
  width: 56px;
  height: 56px;
  border-radius: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24px;
  flex-shrink: 0;
  background: rgba(255,255,255,0.05);
  border: 1px solid rgba(255,255,255,0.1);
  transition: transform 0.3s ease;
}

.metric-card:hover .card-icon-wrapper {
  transform: scale(1.1) rotate(5deg);
}

.user-icon { color: #818cf8; background: rgba(129, 140, 248, 0.15); border-color: rgba(129, 140, 248, 0.3); }
.wallet-icon { color: #fbbf24; background: rgba(251, 191, 36, 0.15); border-color: rgba(251, 191, 36, 0.3); }
.odometer-icon { color: #34d399; background: rgba(52, 211, 153, 0.15); border-color: rgba(52, 211, 153, 0.3); }
.data-icon { color: #60a5fa; background: rgba(96, 165, 250, 0.15); border-color: rgba(96, 165, 250, 0.3); }
.clock-icon { color: #f472b6; background: rgba(244, 114, 182, 0.15); border-color: rgba(244, 114, 182, 0.3); }
.key-icon { color: #a78bfa; background: rgba(167, 139, 250, 0.15); border-color: rgba(167, 139, 250, 0.3); }

.card-content { flex: 1; min-width: 0; }
.card-label {
  font-size: 13px;
  color: #94a3b8;
  text-transform: uppercase;
  letter-spacing: 1px;
  margin-bottom: 6px;
  font-weight: 600;
}
.card-value {
  font-size: 26px;
  font-weight: 800;
  color: #f8fafc;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.quota-bar {
  height: 6px;
  background: rgba(255, 255, 255, 0.05);
  border-radius: 3px;
  margin-top: 10px;
  overflow: hidden;
  box-shadow: inset 0 1px 3px rgba(0,0,0,0.2);
}

.quota-progress {
  height: 100%;
  background: linear-gradient(90deg, #3b82f6, #8b5cf6);
  border-radius: 3px;
  box-shadow: 0 0 10px rgba(139, 92, 246, 0.5);
  transition: width 1s cubic-bezier(0.4, 0, 0.2, 1);
  position: relative;
}
.quota-progress::after {
  content: '';
  position: absolute;
  top: 0; right: 0; bottom: 0; left: 0;
  background: linear-gradient(90deg, transparent, rgba(255,255,255,0.4), transparent);
  animation: shimmer 2s infinite;
}

@keyframes shimmer {
  0% { transform: translateX(-100%); }
  100% { transform: translateX(100%); }
}

.remaining-percent {
  font-size: 13px;
  color: #34d399;
  margin-top: 6px;
  font-weight: 600;
}

.api-key-display {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 6px;
}
.api-key-text {
  font-family: 'Fira Code', 'Courier New', monospace;
  font-size: 14px;
  color: #cbd5e1;
  background: rgba(0,0,0,0.2);
  padding: 4px 8px;
  border-radius: 6px;
  flex: 1;
}

/* Sections */
.middle-section {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 24px;
}
@media (max-width: 900px) {
  .middle-section { grid-template-columns: 1fr; }
}

.models-section, .redeem-section, .logs-section, .guide-section {
  padding: 28px;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 24px;
}
.section-icon {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: rgba(255,255,255,0.1);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  color: #a5b4fc;
  box-shadow: inset 0 0 0 1px rgba(255,255,255,0.1);
}
.present-icon { color: #fca5a5; background: rgba(248, 113, 113, 0.15); }
.log-icon { color: #818cf8; background: rgba(99, 102, 241, 0.15); }
.guide-icon { color: #fcd34d; background: rgba(251, 191, 36, 0.15); }

.section-title {
  font-size: 20px;
  font-weight: 700;
  margin: 0;
  color: #f1f5f9;
}

.models-container {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}
.premium-tag {
  background: linear-gradient(135deg, rgba(16, 185, 129, 0.2) 0%, rgba(52, 211, 153, 0.1) 100%);
  border: 1px solid rgba(16, 185, 129, 0.4);
  color: #34d399;
  padding: 8px 16px;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 600;
  box-shadow: 0 2px 10px rgba(16, 185, 129, 0.1);
}

.models-empty {
  color: #cbd5e1;
  font-size: 15px;
  padding: 16px;
  background: rgba(255,255,255,0.05);
  border-radius: 8px;
}

/* Redeem Section */
.highlight-panel {
  background: linear-gradient(135deg, rgba(30,35,60,0.8) 0%, rgba(40,25,50,0.8) 100%);
  border: 1px solid rgba(139, 92, 246, 0.3);
  position: relative;
}
.highlight-panel::after {
  content: '';
  position: absolute;
  top: 0; right: 0; width: 100px; height: 100px;
  background: radial-gradient(circle, rgba(139,92,246,0.2) 0%, transparent 70%);
  pointer-events: none;
}
.redeem-subtitle {
  margin: 6px 0 0;
  font-size: 14px;
  color: #94a3b8;
}
.redeem-input-wrapper {
  display: flex;
  gap: 12px;
}
.custom-input :deep(.el-input__wrapper) {
  background: rgba(0,0,0,0.2);
  box-shadow: 0 0 0 1px rgba(255,255,255,0.1) inset;
  border-radius: 10px;
  padding: 4px 12px;
}
.custom-input :deep(.el-input__wrapper:hover), .custom-input :deep(.el-input__wrapper.is-focus) {
  box-shadow: 0 0 0 1px #8b5cf6 inset;
}
.custom-input :deep(.el-input__inner) {
  color: #f8fafc;
}
.custom-input :deep(.el-icon) {
  color: #94a3b8;
}
.glow-btn {
  background: linear-gradient(135deg, #6366f1, #8b5cf6);
  border: none;
  color: white;
  box-shadow: 0 4px 15px rgba(99, 102, 241, 0.4);
}
.glow-btn:hover {
  box-shadow: 0 6px 20px rgba(99, 102, 241, 0.6);
  background: linear-gradient(135deg, #4f46e5, #7c3aed);
}

/* Table Style */
.premium-table {
  background: transparent !important;
  border-radius: 12px;
  overflow: hidden;
}
.premium-table :deep(.el-table__inner-wrapper::before) {
  display: none;
}
.premium-table :deep(th.el-table__cell) {
  background: rgba(15, 23, 42, 0.8) !important;
  border-bottom: 1px solid rgba(255,255,255,0.05) !important;
  color: #94a3b8;
  font-weight: 600;
  padding: 12px 0;
}
.premium-table :deep(td.el-table__cell) {
  border-bottom: 1px solid rgba(255,255,255,0.03) !important;
  background: transparent !important;
  padding: 14px 0;
  color: #cbd5e1;
}
.premium-table :deep(tr:hover > td.el-table__cell) {
  background: rgba(255,255,255,0.03) !important;
}

.model-name-badge { color: #f472b6; font-family: monospace; font-size: 13px; background: rgba(244,114,182,0.1); padding: 2px 6px; border-radius: 4px; }
.provider-badge { color: #60a5fa; font-size: 13px; border: 1px solid rgba(96,165,250,0.3); padding: 2px 6px; border-radius: 4px; }
.token-val { font-family: monospace; font-size: 13px; }
.input-token { color: #34d399; }
.output-token { color: #fbbf24; }
.total-token { color: #a78bfa; font-weight: bold; }
.cost-val { color: #ef4444; font-family: monospace; font-size: 13px; }
.time-val { color: #94a3b8; font-size: 12px; }

/* Pagination */
.custom-pagination {
  margin-top: 20px;
  justify-content: flex-end;
}
.custom-pagination :deep(.btn-prev),
.custom-pagination :deep(.btn-next),
.custom-pagination :deep(.el-pager li) {
  background: rgba(255,255,255,0.05) !important;
  border: 1px solid rgba(255,255,255,0.1);
  color: #94a3b8;
  border-radius: 6px;
}
.custom-pagination :deep(.el-pager li.is-active) {
  background: #6366f1 !important;
  border-color: #6366f1;
  color: white;
  box-shadow: 0 2px 8px rgba(99,102,241,0.4);
}

/* Guide Tabs */
.custom-tabs :deep(.el-tabs__item) {
  color: #94a3b8;
  font-size: 15px;
}
.custom-tabs :deep(.el-tabs__item.is-active) {
  color: #a5b4fc;
  font-weight: 600;
}
.custom-tabs :deep(.el-tabs__active-bar) {
  background-color: #a5b4fc;
  height: 3px;
  border-radius: 3px;
}
.custom-tabs :deep(.el-tabs__nav-wrap::after) {
  background-color: rgba(255,255,255,0.05);
}

.guide-block {
  background: rgba(0, 0, 0, 0.2);
  border: 1px solid rgba(255,255,255,0.05);
  border-radius: 12px;
  padding: 24px;
  margin-top: 16px;
}
.guide-text {
  margin: 0 0 16px;
  color: #cbd5e1;
  font-size: 14px;
  line-height: 1.6;
}
.guide-step {
  color: #f1f5f9;
  font-size: 15px;
  font-weight: 600;
  margin: 20px 0 12px;
  display: flex;
  align-items: center;
}
.guide-step::before {
  content: '';
  display: inline-block;
  width: 6px; height: 6px;
  background: #6366f1;
  border-radius: 50%;
  margin-right: 10px;
  box-shadow: 0 0 8px #6366f1;
}

.guide-code-line {
  margin-top: 12px;
  padding: 14px 16px;
  border-radius: 10px;
  background: #0f172a;
  border: 1px solid rgba(99,102,241,0.2);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.guide-code-line code {
  font-family: 'Fira Code', 'Courier New', monospace;
  font-size: 14px;
  color: #34d399;
  word-break: break-all;
  flex: 1;
}
.copy-btn-small {
  width: 32px; height: 32px;
  padding: 0;
  border: none;
  border-radius: 8px;
  transition: all 0.2s;
  display: flex;
  align-items: center;
  justify-content: center;
}
.copy-btn-small:hover {
  background: rgba(255,255,255,0.2) !important;
  color: white;
}

@keyframes fadeIn {
  from { opacity: 0; transform: translateY(20px); }
  to { opacity: 1; transform: translateY(0); }
}

@media (max-width: 768px) {
  .main-content { padding: 16px; }
  .header-section { padding: 24px; }
  .page-title { font-size: 28px; }
  .redeem-input-wrapper { flex-direction: column; }
}
</style>
