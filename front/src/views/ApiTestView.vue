<template>

  <div class="api-test-wrapper">
    <div class="container">
      <header class="header">
        <h1>New API 特殊调用测试</h1>
        <p>测试 OpenAI/Anthropic/Google 的一些特殊调用方式</p>
        <p class="subnote">纯前端网页，所有信息仅存储在您的浏览器本地</p>
        <p class="subnote github-link">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
            <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
          </svg>
          <a href="https://github.com/CookSleep/newapi-special-test" target="_blank" rel="noopener">@CookSleep</a>
        </p>
      </header>

      <main class="main-card">
        <section class="config-section">
          <div class="config-header">
            <h2>API 配置</h2>
            <div class="btn-group">
              <button class="btn btn-secondary" id="manageConfigBtn">管理配置</button>
            </div>
          </div>

          <div class="grid-3 input-row">
            <div class="input-group">
              <label for="apiUrl">API URL</label>
              <input type="text" id="apiUrl" placeholder="例如：https://api.openai.com" />
            </div>
            <div class="input-group">
              <label for="apiKey">API Key</label>
              <div class="password-input-wrapper">
                <input type="password" id="apiKey" placeholder="例如：sk-xxxxxxxxxxxx" />
                <button type="button" class="password-toggle" data-target="apiKey" aria-label="显示/隐藏密码">
                  <svg class="eye-icon" width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M12 4.5C7 4.5 2.73 7.61 1 12c1.73 4.39 6 7.5 11 7.5s9.27-3.11 11-7.5c-1.73-4.39-6-7.5-11-7.5zM12 17c-2.76 0-5-2.24-5-5s2.24-5 5-5 5 2.24 5 5-2.24 5-5 5zm0-8c-1.66 0-3 1.34-3 3s1.34 3 3 3 3-1.34 3-3-1.34-3-3-3z"/>
                  </svg>
                  <svg class="eye-off-icon" width="16" height="16" viewBox="0 0 24 24" fill="currentColor" style="display: none;">
                    <path d="M12 7c2.76 0 5 2.24 5 5 0 .65-.13 1.26-.36 1.83l2.92 2.92c1.51-1.26 2.7-2.89 3.43-4.75-1.73-4.39-6-7.5-11-7.5-1.4 0-2.74.25-3.98.7l2.16 2.16C10.74 7.13 11.35 7 12 7zM2 4.27l2.28 2.28.46.46C3.08 8.3 1.78 10.02 1 12c1.73 4.39 6 7.5 11 7.5 1.55 0 3.03-.3 4.38-.84l.42.42L19.73 22 21 20.73 3.27 3 2 4.27zM7.53 9.8l1.55 1.55c-.05.21-.08.43-.08.65 0 1.66 1.34 3 3 3 .22 0 .44-.03.65-.08l1.55 1.55c-.67.33-1.41.53-2.2.53-2.76 0-5-2.24-5-5 0-.79.2-1.53.53-2.2zm4.31-.78l3.15 3.15.02-.16c0-1.66-1.34-3-3-3l-.17.01z"/>
                  </svg>
                </button>
              </div>
            </div>
            <div class="input-group">
              <label for="model">模型</label>
              <input type="text" id="model" placeholder="例如：gpt-4o" />
            </div>
          </div>
        </section>

        <section class="test-section">
          <h2>测试内容</h2>
          <p class="muted">在下方选择测试类型，编辑用户消息，然后发送请求。结果将按轮次与请求分块展示。</p>

          <div class="segmented" id="vendorType">
            <button class="seg-btn active" data-vendor="openai">OpenAI</button>
            <button class="seg-btn" data-vendor="anthropic">Anthropic</button>
            <button class="seg-btn" data-vendor="google">Google</button>
          </div>
          <div class="segmented" id="testType">
            <!-- 由JS动态渲染 -->
          </div>

          <div class="input-group">
            <label for="userInput">用户消息</label>
            <textarea id="userInput" class="input-textarea" rows="3" placeholder="请输入本次测试的用户消息"></textarea>
          </div>
          <div class="btn-row">
            <button class="btn btn-primary" id="testBtn">发送测试请求</button>
            <button class="btn" id="clearBtn">清空结果</button>
          </div>

          <div class="split-columns">
            <div class="column">
              <h3>轮次与消息</h3>
              <div id="messageTimeline" class="timeline"></div>
            </div>
            <div class="column">
              <h3>请求与响应</h3>
              <div id="blocksContainer" class="blocks-container"></div>
            </div>
          </div>

          <div id="errorMessage" class="error-message"></div>
        </section>
      </main>
    </div>

    <!-- 配置管理模态框 -->
    <div id="configModal" class="modal">
      <div class="modal-content">
        <div class="modal-header">
          <h2 id="configModalTitle">管理 API 配置</h2>
          <span class="close" id="closeConfigModal">&times;</span>
        </div>

        <div class="config-modal-body">
          <div class="modal-left">
            <h3 id="modalFormHeading" class="subheading">添加新配置</h3>

            <div class="input-group">
              <label for="configName">配置名称</label>
              <input type="text" id="configName" placeholder="例如：生产环境" />
            </div>

            <div class="input-group">
              <label for="configUrl">API URL</label>
              <input type="text" id="configUrl" placeholder="例如：https://api.openai.com" />
            </div>

            <div class="input-group">
              <label for="configKey">API Key</label>
              <div class="password-input-wrapper">
                <input type="password" id="configKey" placeholder="例如：sk-xxxxxxxxxxxx" />
                <button type="button" class="password-toggle" data-target="configKey" aria-label="显示/隐藏密码">
                  <svg class="eye-icon" width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M12 4.5C7 4.5 2.73 7.61 1 12c1.73 4.39 6 7.5 11 7.5s9.27-3.11 11-7.5c-1.73-4.39-6-7.5-11-7.5zM12 17c-2.76 0-5-2.24-5-5s2.24-5 5-5 5 2.24 5 5-2.24 5-5 5zm0-8c-1.66 0-3 1.34-3 3s1.34 3 3 3 3-1.34 3-3-1.34-3-3-3z"/>
                  </svg>
                  <svg class="eye-off-icon" width="16" height="16" viewBox="0 0 24 24" fill="currentColor" style="display: none;">
                    <path d="M12 7c2.76 0 5 2.24 5 5 0 .65-.13 1.26-.36 1.83l2.92 2.92c1.51-1.26 2.7-2.89 3.43-4.75-1.73-4.39-6-7.5-11-7.5-1.4 0-2.74.25-3.98.7l2.16 2.16C10.74 7.13 11.35 7 12 7zM2 4.27l2.28 2.28.46.46C3.08 8.3 1.78 10.02 1 12c1.73 4.39 6 7.5 11 7.5 1.55 0 3.03-.3 4.38-.84l.42.42L19.73 22 21 20.73 3.27 3 2 4.27zM7.53 9.8l1.55 1.55c-.05.21-.08.43-.08.65 0 1.66 1.34 3 3 3 .22 0 .44-.03.65-.08l1.55 1.55c-.67.33-1.41.53-2.2.53-2.76 0-5-2.24-5-5 0-.79.2-1.53.53-2.2zm4.31-.78l3.15 3.15.02-.16c0-1.66-1.34-3-3-3l-.17.01z"/>
                  </svg>
                </button>
              </div>
            </div>

            <div class="input-group">
              <label for="configModel">默认模型</label>
              <input type="text" id="configModel" placeholder="例如：gpt-4o" />
            </div>

            <div class="modal-actions">
              <button class="btn btn-primary" id="saveAsDefaultBtn">保存为默认配置</button>
              <button class="btn btn-secondary" id="saveConfigBtn">保存配置</button>
              <button class="btn btn-secondary" id="cancelEditBtn" style="display:none;">取消编辑</button>
            </div>
          </div>

          <div class="modal-right">
            <h3 class="subheading">已有配置</h3>
            <div class="config-list" id="configList"></div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { onMounted, onBeforeUnmount } from 'vue'

// 系统默认配置
const APP_CONFIG = {
  apiUrl: 'https://api.openai.com',
  apiKey: '',
  model: 'gemini-2.5-pro'
}

// 将配置注入到 window 对象供原脚本使用
if (typeof window !== 'undefined') {
  window.APP_CONFIG = APP_CONFIG
}

let scriptElement = null

onMounted(() => {
  // 动态加载原始 script.js
  scriptElement = document.createElement('script')
  scriptElement.src = '/api-test-script.js'
  scriptElement.async = true
  document.body.appendChild(scriptElement)
})

onBeforeUnmount(() => {
  // 清理脚本
  if (scriptElement && scriptElement.parentNode) {
    scriptElement.parentNode.removeChild(scriptElement)
  }
})
</script>

<style scoped>
* {
  box-sizing: border-box;
}

html,
body {
  height: 100%;
  overflow-x: hidden;
}

body {
  margin: 0;
  font-family: system-ui, sans-serif;
  background: linear-gradient(135deg, #e6f2ff 0%, #ffffff 100%);
  color: #1f2937;
}

/* 自定义滚动条（全局） */
:root {
  --sb-track: #eef6ff;
  --sb-thumb: #cfe6ff;
  --sb-thumb-hover: #9cc8ff;
}

/* Firefox 浏览器 */
* {
  scrollbar-color: var(--sb-thumb) var(--sb-track);
  scrollbar-width: thin;
}

/* WebKit 内核 (Chrome/Edge/Safari) */
*::-webkit-scrollbar {
  width: 10px;
  height: 10px;
}

*::-webkit-scrollbar-track {
  background: var(--sb-track);
  border-radius: 8px;
}

*::-webkit-scrollbar-thumb {
  background-color: var(--sb-thumb);
  border-radius: 8px;
  border: 2px solid var(--sb-track);
}

*::-webkit-scrollbar-thumb:hover {
  background-color: var(--sb-thumb-hover);
}

*::-webkit-scrollbar-corner {
  background: transparent;
}

.container {
  max-width: 1200px;
  margin: 0 auto;
  padding: 24px;
}

@media (max-width: 600px) {
  .container {
    padding: 12px;
  }
}

.header {
  text-align: center;
  color: #0f172a;
  margin-bottom: 32px;
}

.header h1 {
  font-size: 28px;
  margin: 8px 0;
}

.header p {
  color: #475569;
}

.header .subnote {
  color: #6b7280;
  font-size: 12px;
  margin-top: 4px;
}

.header .subnote a {
  color: #1e90ff;
  text-decoration: none;
  font-weight: 600;
}

.header .subnote a:hover {
  color: #1778d6;
  text-decoration: underline;
}

.header .github-link {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
}

.header .github-link svg {
  color: #6b7280;
}

.main-card {
  background: #fff;
  border-radius: 16px;
  padding: 28px 24px 24px;
  box-shadow: 0 12px 24px rgba(30, 144, 255, 0.12);
  border: 1px solid #e5f1ff;
}

@media (max-width: 600px) {
  .main-card {
    padding: 16px;
    border-radius: 12px;
  }
}

.config-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.config-header h2 {
  margin: 0;
  font-size: 18px;
  color: #0f172a;
}

/* 统一二级与三级标题字号与样式 */
.test-section h2 {
  margin: 12px 0 18px;
  font-size: 18px;
  color: #0f172a;
}

.column h3 {
  margin: 16px 0 16px;
  color: #0f172a;
  font-size: 18px;
}

/* 分区之间增加垂直间距 */
.test-section {
  margin-top: 36px;
}

.config-section {
  margin-bottom: 36px;
}

.input-row {
  margin-top: 16px;
}

.grid-3 {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 20px;
}

@media (max-width: 900px) {
  .grid-3 {
    grid-template-columns: 1fr;
    gap: 12px;
  }
}

.input-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.input-group label {
  font-weight: 600;
  color: #334155;
}

.input-group input,
.input-group select {
  padding: 12px 14px;
  border: 1.5px solid #cfe6ff;
  border-radius: 10px;
  font-size: 15px;
  outline: none;
  transition: border-color .2s ease;
}

.input-group input:focus,
.input-group select:focus {
  border-color: #1e90ff;
}

/* 密码输入框包装器 */
.password-input-wrapper {
  position: relative;
  display: flex;
  align-items: center;
}

.password-input-wrapper input {
  padding-right: 40px;
  flex: 1;
}

.password-toggle {
  position: absolute;
  right: 10px;
  top: 50%;
  transform: translateY(-50%);
  background: none;
  border: none;
  cursor: pointer;
  color: #6b7280;
  padding: 4px;
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  z-index: 10;
}

.password-toggle:hover {
  color: #1e90ff;
  background: #f8fbff;
}

.password-toggle:focus {
  outline: 2px solid #1e90ff;
  outline-offset: 1px;
}

.btn-row {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  margin: 14px 0 24px;
}

.btn-group {
  display: flex;
  gap: 10px;
}

.btn {
  padding: 10px 16px;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  background: #f1f5f9;
  color: #0f172a;
  font-weight: 600;
}

.btn:hover {
  background: #e2e8f0;
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
  pointer-events: none;
}

.btn-primary {
  background: #1e90ff;
  color: #fff;
}

.btn-primary:hover {
  background: #1778d6;
}

.btn-secondary {
  background: #e7f2ff;
  color: #0f172a;
  border: 1px solid #cfe6ff;
}

.btn-secondary:hover {
  background: #d9ecff;
}

.muted {
  color: #64748b;
  margin: 6px 0 18px;
}

/* 模型厂商页签 */
#vendorType {
  display: flex;
  gap: 24px;
  border-bottom: 2px solid #eef6ff;
  margin-bottom: 20px;
  padding: 0 4px;
  overflow-x: auto;
  scrollbar-width: none;
}

#vendorType::-webkit-scrollbar {
  display: none;
}

@media (max-width: 600px) {
  #vendorType {
    gap: 16px;
  }
}

#vendorType .seg-btn {
  background: transparent;
  border: none;
  border-radius: 0;
  padding: 10px 4px;
  color: #64748b;
  font-weight: 600;
  position: relative;
  transition: all 0.2s ease;
}

#vendorType .seg-btn::after {
  content: '';
  position: absolute;
  bottom: -2px;
  left: 0;
  width: 100%;
  height: 2px;
  background: #1e90ff;
  transform: scaleX(0);
  transition: transform 0.2s ease;
}

#vendorType .seg-btn:hover {
  background: transparent;
  color: #1e90ff;
}

#vendorType .seg-btn.active {
  background: transparent;
  color: #1e90ff;
  border: none;
}

#vendorType .seg-btn.active::after {
  transform: scaleX(1);
}

/* 测试场景胶囊按钮 */
#testType {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-bottom: 20px;
}

#testType .seg-btn {
  border: 1px solid #e2e8f0;
  background: #ffffff;
  color: #475569;
  padding: 6px 16px;
  border-radius: 10px;
  font-size: 13px;
  font-weight: 500;
  transition: all 0.2s ease;
}

#testType .seg-btn:hover {
  border-color: #cbd5e1;
  background: #f8fafc;
  color: #334155;
}

#testType .seg-btn.active {
  background: #eff6ff;
  border-color: #bfdbfe;
  color: #1e90ff;
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(30, 144, 255, 0.05);
}

/* 其他用途的通用分段控件 */
.segmented {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 6px 0 12px;
}

.segmented:not(#vendorType):not(#testType) .seg-btn {
  border: 1px solid #cfe6ff;
  background: #ffffff;
  color: #0f172a;
  padding: 8px 12px;
  border-radius: 10px;
  cursor: pointer;
  font-weight: 600;
}

.segmented:not(#vendorType):not(#testType) .seg-btn:hover {
  background: #f4f9ff;
}

.segmented:not(#vendorType):not(#testType) .seg-btn.active {
  background: #1e90ff;
  color: #ffffff;
  border-color: #1e90ff;
}

/* 用户输入文本框 */
.input-textarea {
  width: 100%;
  padding: 12px 14px;
  border: 1.5px solid #cfe6ff;
  border-radius: 10px;
  font-size: 15px;
  line-height: 1.6;
  resize: vertical;
  min-height: 80px;
}

.input-textarea:focus {
  outline: none;
  border-color: #1e90ff;
}

.split-columns {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 20px;
  margin-top: 12px;
}

@media (max-width: 600px) {
  .split-columns {
    gap: 16px;
  }
}

.split-columns>.column {
  min-width: 0;
  overflow: hidden;
}

@media (max-width: 900px) {
  .split-columns {
    grid-template-columns: 1fr;
  }
}

.blocks-container,
.timeline {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
}

.card,
.code-block {
  background: #ffffff;
  border: 1px solid #e5f1ff;
  border-radius: 12px;
  padding: 14px;
  position: relative;
  max-width: 100%;
}

.card .title,
.code-block .title {
  font-size: 13px;
  font-weight: 700;
  color: #0f172a;
  margin-bottom: 10px;
  letter-spacing: .2px;
}

.code-block pre,
.card pre {
  margin: 0;
  white-space: pre;
  overflow-x: auto;
  max-width: 100%;
  font-family: ui-monospace, monospace;
  font-size: 13px;
  line-height: 1.6;
  color: #0b2542;
  background: #f8fbff;
  border: 1px solid #e5f1ff;
  border-radius: 8px;
  padding: 10px;
}

.copy-btn {
  position: absolute;
  right: 10px;
  top: 10px;
  font-size: 12px;
  padding: 4px 8px;
  border-radius: 6px;
  background: #ffffff;
  border: 1px solid #cfe6ff;
  cursor: pointer;
}

.copy-btn:hover {
  background: #f0f7ff;
}

.error-message {
  color: #dc2626;
  margin-top: 12px;
}

.error-inline {
  background: #fff1f2;
  border: 1px solid #fecdd3;
  color: #b91c1c;
  border-radius: 10px;
  padding: 10px 12px;
  margin: 8px 0;
  font-size: 13px;
}

.error-inline pre {
  font-family: ui-monospace, monospace;
  font-size: 13px;
}

.info-inline {
  background: #fffbeb;
  border: 1px solid #fde68a;
  color: #92400e;
  border-radius: 10px;
  padding: 10px 12px;
  margin: 8px 0;
  font-size: 13px;
}

.success-inline {
  background: #f0fdf4;
  border: 1px solid #bbf7d0;
  color: #166534;
  border-radius: 10px;
  padding: 10px 12px;
  margin: 8px 0;
  font-size: 13px;
}

.success-message {
  color: #059669;
  margin-top: 8px;
  display: none;
}

/* 对话时间轴消息样式 */
.message {
  border-left: 4px solid #cfe6ff;
  padding-left: 10px;
}

.message.user {
  border-left-color: #1e90ff;
}

.message.assistant {
  border-left-color: #22c55e;
}

.message.tool {
  border-left-color: #7c3aed;
}

.message .meta {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 6px;
  color: #334155;
  font-size: 12px;
}

.message .role {
  font-weight: 700;
}

.message .label {
  opacity: .8;
}

/* 最新消息下方的内联加载等待器 */
.waiting-inline {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  margin: 8px 0 4px;
  padding-left: 10px;
}

.waiting-inline .label {
  color: #64748b;
  font-size: 12px;
  user-select: none;
}

.waiting-inline .dots {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.waiting-inline .dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #cbd5e1;
  animation: dotPulse 1.2s ease-in-out infinite;
}

.waiting-inline .dot:nth-child(1) {
  animation-delay: 0s;
}

.waiting-inline .dot:nth-child(2) {
  animation-delay: .15s;
}

.waiting-inline .dot:nth-child(3) {
  animation-delay: .3s;
}

@keyframes dotPulse {

  0%,
  100% {
    background: #cbd5e1;
    transform: scale(.85);
  }

  50% {
    background: #475569;
    transform: scale(1);
  }
}

/* 弹窗 */
.modal {
  display: none;
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.65);
  backdrop-filter: blur(4px);
  z-index: 1000;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transition: opacity 0.2s ease;
}

.modal.open {
  display: flex;
  opacity: 1;
}

.modal-content {
  background: #ffffff;
  width: 92%;
  max-width: 900px;
  margin: 0;
  padding: 0;
  border-radius: 20px;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.2);
  border: 1px solid rgba(255, 255, 255, 0.1);
  max-height: 85vh;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  transform: scale(0.95);
  transition: transform 0.2s ease;
}

@media (max-width: 600px) {
  .modal-content {
    width: 96%;
    max-height: 94vh;
    border-radius: 16px;
  }
}

.modal.open .modal-content {
  transform: scale(1);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 24px 32px;
  border-bottom: 1px solid #f1f5f9;
  background: #ffffff;
}

@media (max-width: 600px) {
  .modal-header {
    padding: 16px 20px;
  }
}

.modal-header h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
  color: #1e293b;
}

@media (max-width: 600px) {
  .modal-header h2 {
    font-size: 18px;
  }
}

.close {
  color: #94a3b8;
  font-size: 28px;
  font-weight: 400;
  cursor: pointer;
  line-height: 1;
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  transition: all 0.2s ease;
}

.close:hover {
  color: #ef4444;
  background: #fee2e2;
}

.config-modal-body {
  display: grid;
  grid-template-columns: 1fr 320px;
  flex: 1;
  min-height: 0;
  background: #f8fafc;
}

@media (max-width: 900px) {
  .config-modal-body {
    grid-template-columns: 1fr;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
  }
}

.modal-left {
  padding: 32px;
  overflow-y: auto;
  background: #ffffff;
}

@media (max-width: 900px) {
  .modal-left {
    overflow-y: visible;
    padding: 20px;
    flex-shrink: 0;
  }
}

.modal-right {
  display: flex;
  flex-direction: column;
  min-height: 0;
  background: #f8fafc;
  border-left: 1px solid #e2e8f0;
  padding: 24px;
}

@media (max-width: 900px) {
  .modal-right {
    border-left: none;
    border-top: 1px solid #e2e8f0;
    padding: 20px;
    flex-shrink: 0;
    min-height: 300px;
  }
}

.subheading {
  margin: 0 0 20px;
  color: #334155;
  font-size: 14px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.config-list {
  flex: 1;
  overflow-y: auto;
  margin-bottom: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 4px 8px 4px 4px;
}

@media (max-width: 900px) {
  .config-list {
    overflow-y: visible;
  }
}

.config-item {
  padding: 16px;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  cursor: pointer;
  background: #ffffff;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.02);
}

.config-item:hover {
  background: #ffffff;
  border-color: #93c5fd;
  transform: translateY(-2px);
  box-shadow: 0 6px 16px rgba(30, 144, 255, 0.1);
}

.config-item.active {
  border-color: #3b82f6;
  background: #eff6ff;
}

.config-info {
  flex: 1;
  min-width: 0;
  padding-right: 12px;
}

.config-name {
  font-weight: 600;
  color: #1e293b;
  margin-bottom: 6px;
  font-size: 14px;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
}

/* 默认配置徽章 */
.badge-default {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  background: #eff6ff;
  color: #3b82f6;
  border: 1px solid #bfdbfe;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 600;
  line-height: 1.4;
  letter-spacing: 0.5px;
}

.config-url {
  color: #64748b;
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-family: ui-monospace, monospace;
}

.config-actions {
  display: flex;
  align-items: center;
  gap: 2px;
}

.icon-btn {
  width: 26px;
  height: 26px;
  padding: 0;
  background: transparent;
  border: none;
  color: #94a3b8;
  border-radius: 6px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s ease;
}

.icon-btn:hover {
  background: #f1f5f9;
  color: #475569;
}

.icon-btn svg {
  width: 14px;
  height: 14px;
}

.star-config:hover {
  color: #f59e0b;
  background: #fffbeb;
}

.star-config.starred {
  color: #f59e0b;
}

.delete-config {
  position: relative;
}

.delete-config:hover {
  color: #ef4444;
  background: #fef2f2;
}

.delete-config .q-badge {
  position: absolute;
  right: -2px;
  top: -2px;
  width: 12px;
  height: 12px;
  background: #ef4444;
  color: #fff;
  font-size: 9px;
  border-radius: 50%;
  display: none;
  align-items: center;
  justify-content: center;
  font-weight: bold;
}

.delete-config.confirm {
  color: #ef4444;
  background: #fee2e2;
}

.delete-config.confirm .q-badge {
  display: flex;
}

/* 弹窗输入框 */
.modal .input-group {
  margin-bottom: 20px;
}

.modal .input-group label {
  font-size: 13px;
  font-weight: 500;
  color: #64748b;
  margin-bottom: 8px;
  display: block;
}

.modal .input-group input {
  width: 100%;
  padding: 12px 16px;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  font-size: 14px;
  background: #f8fafc;
  transition: all 0.2s ease;
}

.modal .input-group input:focus {
  background: #ffffff;
  border-color: #3b82f6;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
}

.modal-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 32px;
  padding-top: 24px;
  border-top: 1px solid #f1f5f9;
  flex-wrap: wrap;
}

@media (max-width: 600px) {
  .modal-actions {
    gap: 8px;
    margin-top: 20px;
    padding-top: 16px;
  }

  .modal-actions .btn {
    flex: 1;
    min-width: 120px;
  }
}

.modal-actions .btn {
  padding: 10px 20px;
  font-size: 14px;
}

/* 应用提示弹窗（自定义提示） */
.app-modal {
  display: none;
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.35);
  z-index: 1100;
  align-items: center;
  justify-content: center;
}

.app-modal.open {
  display: flex;
}

.app-modal .box {
  width: 92%;
  max-width: 420px;
  margin: 0;
  padding: 18px;
  border-radius: 12px;
  background: #ffffff;
  border: 1px solid #e5f1ff;
  box-shadow: 0 10px 24px rgba(30, 144, 255, 0.18);
  animation: modalPop .18s ease-out;
  max-height: 70vh;
  overflow: auto;
}

.app-modal .title {
  font-size: 16px;
  font-weight: 700;
  color: #0f172a;
  margin-bottom: 8px;
}

.app-modal .content {
  color: #334155;
  line-height: 1.6;
  white-space: pre-wrap;
}

.app-modal .actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 14px;
}

.app-modal .actions .btn {
  padding: 8px 14px;
  border-radius: 8px;
  font-weight: 600;
  cursor: pointer;
  border: 1px solid #cfe6ff;
  background: #e7f2ff;
  color: #0f172a;
}

.app-modal .actions .btn:hover {
  background: #d9ecff;
}

.app-modal .actions .btn-primary {
  background: #1e90ff;
  color: #fff;
  border-color: #1e90ff;
}

.app-modal .actions .btn-primary:hover {
  background: #1778d6;
}

/* 全局 Toast 提示 */
.toast-container {
  position: fixed;
  top: 24px;
  left: 50%;
  transform: translateX(-50%) translateY(-100px);
  z-index: 2000;
  pointer-events: none;
  transition: transform 0.4s cubic-bezier(0.18, 0.89, 0.32, 1.28), opacity 0.3s ease;
  opacity: 0;
}

.toast-container.show {
  transform: translateX(-50%) translateY(0);
  opacity: 1;
}

.toast {
  background: rgba(255, 255, 255, 0.98);
  backdrop-filter: blur(10px);
  color: #1e293b;
  padding: 14px 28px;
  border-radius: 12px;
  box-shadow: 0 12px 30px rgba(0, 0, 0, 0.12);
  border: 2px solid #e2e8f0;
  font-weight: 600;
  font-size: 14px;
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 220px;
  justify-content: center;
  overflow: hidden;
}

.toast.error {
  border-color: #f87171;
  background: rgba(254, 242, 242, 0.98);
  color: #991b1b;
}

.toast.success {
  border-color: #34d399;
  background: rgba(240, 253, 244, 0.98);
  color: #065f46;
}

.toast.info {
  border-color: #60a5fa;
  background: rgba(239, 246, 255, 0.98);
  color: #1e40af;
}

@keyframes modalPop {
  from {
    transform: translateY(4px);
    opacity: .6;
  }

  to {
    transform: translateY(0);
    opacity: 1;
  }
}

/* 确保样式不会被 Vue 的 scoped 影响全局元素 */
.api-test-wrapper {
  min-height: 100vh;
}
</style>
