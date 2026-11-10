<template>
  <section class="surface mt-4 p-4 sm:p-5 md:p-6" aria-label="API Token 管理">
    <div class="mb-4 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h2 class="panel-title"><span class="panel-icon"><i class="ri-key-2-line" aria-hidden="true"></i></span>API Token 管理</h2>
        <p class="mt-2 text-sm text-secondary">按应用创建独立凭据，权限互不影响。每个账户最多保留 {{ maxActive }} 个未到期的 Token（含已暂停的）。</p>
      </div>
      <button type="button" class="soft-button" :disabled="loading" @click="loadTokens(true)">
        <i class="ri-refresh-line mr-1" aria-hidden="true"></i>刷新
      </button>
    </div>
    <div v-if="!apiEnabled" class="mb-4 rounded-lg border border-amber-300/60 p-3 text-sm text-amber-700 dark:text-amber-300" role="note">
      API 总开关当前关闭。创建和管理 Token 不受影响，但要在右侧开启 API 后才能使用。
    </div>

    <!-- This raw secret stays only in the current Vue component's memory. -->
    <div v-if="issuedSecret" class="mb-5 rounded-lg border border-amber-400 p-4" role="status">
      <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
        <strong class="text-sm">{{ issuedAction === 'rotated' ? 'Token 已重新生成' : issuedAction === 'revealed' ? '已读取原 Token（未轮换）' : '新 Token 已创建' }}：{{ issuedTokenName }}，请立即复制并安全保存</strong>
        <span class="text-xs text-amber-700 dark:text-amber-300">完整 Token 已加密保存；关闭或刷新页面后可在列表再次复制，不会轮换密钥</span>
      </div>
      <div class="flex flex-col gap-2 sm:flex-row">
        <input :value="issuedSecret" readonly type="text" aria-label="刚创建的 API Token" class="input-modern min-w-0 flex-1 font-mono text-xs" autocomplete="off" spellcheck="false" @focus="$event.target.select()" />
        <button type="button" class="primary-button" @click="copySecret"><i class="ri-file-copy-line mr-1" aria-hidden="true"></i>{{ copied ? '已复制' : '复制 Token' }}</button>
        <button type="button" class="soft-button" @click="clearSecret">已保存，隐藏</button>
      </div>
    </div>

    <form class="mb-6 space-y-4 rounded-lg border border-slate-200 p-4 dark:border-white/10" @submit.prevent="createToken">
      <h3 class="font-medium">创建 API Token</h3>
      <div class="grid gap-4 sm:grid-cols-2">
        <div>
          <label class="field-label" for="new-token-name">Token 名称</label>
          <input id="new-token-name" v-model.trim="createForm.name" required maxlength="64" class="input-modern" placeholder="例如：ScriptCat 图片上传" />
        </div>
        <div>
          <label class="field-label" for="new-token-description">备注（选填）</label>
          <input id="new-token-description" v-model.trim="createForm.description" maxlength="200" class="input-modern" placeholder="用于区分不同设备或程序" />
        </div>
      </div>

      <fieldset>
        <legend class="field-label">权限范围（按最小权限勾选）</legend>
        <div class="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          <label v-for="scope in scopes" :key="scope.code" class="flex cursor-pointer gap-2 rounded-md border border-slate-200 p-3 text-sm dark:border-white/10">
            <input v-model="createForm.scopes" :value="scope.code" type="checkbox" class="mt-1 shrink-0" />
            <span><strong class="block">{{ scope.name }}</strong><span class="mt-0.5 block text-xs text-secondary">{{ scope.description }}</span><code class="mt-1 block text-xs text-secondary">{{ scope.code }}</code></span>
          </label>
        </div>
        <p class="mt-2 text-xs text-secondary">推荐 ScriptCat 只使用“查看图片＋上传图片”。删除和文件夹写入需要明确授权。</p>
      </fieldset>

      <div class="grid gap-4 sm:grid-cols-2">
        <div>
          <label class="field-label" for="new-token-expiry">到期时间</label>
          <select id="new-token-expiry" v-model="createForm.expiry" class="input-modern">
            <option value="1">1 天</option>
            <option value="7">7 天</option>
            <option value="30">30 天（推荐）</option>
            <option value="90">90 天</option>
            <option value="180">180 天</option>
            <option value="365">365 天</option>
            <option value="long">长期（不过期，可手动撤销）</option>
            <option value="custom">自定义日期时间</option>
          </select>
        </div>
        <div v-if="createForm.expiry === 'custom'">
          <label class="field-label" for="new-token-custom-expiry">自定义到期时间（本地时间）</label>
          <input id="new-token-custom-expiry" v-model="createForm.customExpiry" class="input-modern" type="datetime-local" required />
        </div>
      </div>
      <p v-if="createForm.expiry === 'long'" class="text-xs text-amber-700 dark:text-amber-300" role="note">长期 Token 不会自动过期。请仅授予必要权限，发现泄露时立即暂停或撤销。</p>
      <div class="flex flex-wrap items-center gap-3">
        <button class="primary-button" type="submit" :disabled="busy || !scopes.length || !createForm.scopes.length || !!issuedSecret">{{ busy ? '正在创建…' : '创建 Token' }}</button>
        <span v-if="issuedSecret" class="text-xs text-secondary">先保存并隐藏上方 Token，才能继续创建。</span>
        <span v-else class="text-xs text-secondary">创建不会影响其他 Token；长期密钥泄露风险更高，建议只授权上传所需权限。</span>
      </div>
    </form>

    <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
      <h3 class="font-medium">Token 列表 <span class="ml-1 text-sm font-normal text-secondary">有效 {{ activeCount }} / {{ maxActive }} · 共 {{ total }} 条记录</span></h3>
      <span class="text-xs text-secondary">点击“复制 Token”可重复复制原密钥，不会轮换；“重置密钥”是独立的危险操作</span>
    </div>
    <p v-if="loading && !tokens.length" class="text-sm text-secondary">正在加载 Token…</p>
    <p v-if="!loading && !tokens.length" class="text-sm text-secondary">尚无 Token，请先创建一个。</p>
    <div class="space-y-3">
      <article v-for="token in tokens" :key="token.id" class="rounded-lg border border-slate-200 p-3.5 dark:border-white/10">
        <div class="flex flex-wrap items-start justify-between gap-2">
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <strong class="break-all text-sm">{{ token.name }}</strong>
              <span class="rounded-md border px-2 py-0.5 text-xs" :class="token.status === 'active' ? 'text-emerald-700 dark:text-emerald-400' : 'text-secondary'">{{ statusLabels[token.status] || token.status }}</span>
              <span v-if="token.legacy" class="text-xs text-secondary">历史凭据</span>
            </div>
            <p v-if="token.description" class="mt-1 break-words text-xs text-secondary">{{ token.description }}</p>
            <p v-if="token.token_hint" class="mt-1 font-mono text-xs text-secondary">{{ token.token_hint }}</p>
            <p v-if="!token.legacy && !token.can_reveal && (token.status === 'active' || token.status === 'disabled')" class="mt-1 text-xs text-amber-700 dark:text-amber-300">旧版 Token 未保存加密副本：如需再次复制，请主动“重置密钥”一次，之后可重复复制。</p>
          </div>
          <div class="flex flex-wrap gap-2">
            <button v-if="issuedSecret && token.id === issuedTokenId" type="button" class="primary-button" @click="copySecret"><i class="ri-file-copy-line mr-1" aria-hidden="true"></i>{{ copied ? '已复制' : '复制 Token' }}</button>
            <button v-else-if="token.can_reveal && (token.status === 'active' || token.status === 'disabled')" type="button" class="primary-button" :disabled="busy" title="复制原 Token，不修改密钥或有效期" @click="copyStoredToken(token)"><i class="ri-file-copy-line mr-1" aria-hidden="true"></i>复制 Token</button>
            <button v-if="!token.legacy && (token.status === 'active' || token.status === 'disabled')" type="button" class="soft-button" :disabled="busy || !!issuedSecret" title="安全操作：只在主动确认后重置密钥，原密钥会立即失效" @click="rotateToken(token)"><i class="ri-refresh-line mr-1" aria-hidden="true"></i>重置密钥</button>
            <button v-if="!token.legacy && (token.status === 'active' || token.status === 'disabled')" type="button" class="soft-button" :disabled="busy" @click="beginEdit(token)">编辑</button>
            <button v-if="!token.legacy && (token.status === 'active' || token.status === 'disabled')" type="button" class="soft-button" :disabled="busy" @click="toggleToken(token)">{{ token.disabled ? '恢复' : '暂停' }}</button>
            <button v-if="token.status !== 'revoked'" type="button" class="soft-button" :disabled="busy" @click="revokeToken(token)">撤销</button>
          </div>
        </div>
        <div class="mt-3 flex flex-wrap gap-1.5">
          <code v-for="code in token.scopes" :key="code" class="rounded bg-slate-100 px-1.5 py-0.5 text-xs dark:bg-slate-800">{{ code }}</code>
        </div>
        <div class="mt-2 grid gap-1 text-xs text-secondary sm:grid-cols-3">
          <p>创建：{{ formatDate(token.created_at) }}</p>
          <p>到期：{{ token.never_expires ? '长期（不过期）' : formatDate(token.expires_at) }}</p>
          <p>最近使用：{{ token.last_used_at ? formatDate(token.last_used_at) : '尚未使用' }}</p>
        </div>

        <form v-if="editingId === token.id" class="mt-4 space-y-3 border-t border-slate-200 pt-4 dark:border-white/10" @submit.prevent="saveEdit(token)">
          <h4 class="text-sm font-medium">编辑 {{ token.name }}</h4>
          <div class="grid gap-3 sm:grid-cols-2">
            <label class="text-xs">名称<input v-model.trim="editForm.name" required maxlength="64" class="input-modern mt-1" /></label>
            <label class="text-xs">备注<input v-model.trim="editForm.description" maxlength="200" class="input-modern mt-1" /></label>
          </div>
          <fieldset>
            <legend class="text-xs font-medium">授权范围</legend>
            <div class="mt-2 flex flex-wrap gap-3">
              <label v-for="scope in scopes" :key="scope.code" class="inline-flex items-center gap-1.5 text-xs">
                <input v-model="editForm.scopes" type="checkbox" :value="scope.code" />{{ scope.name }}
              </label>
            </div>
          </fieldset>
          <label class="flex items-center gap-2 text-xs">
            <input v-model="editForm.neverExpires" type="checkbox" />
            长期有效（不过期，可随时暂停或撤销）
          </label>
          <label v-if="!editForm.neverExpires" class="block text-xs">到期时间（本地时间）
            <input v-model="editForm.expiresAt" type="datetime-local" class="input-modern mt-1" required />
          </label>
          <p class="text-xs text-secondary">修改权限将立即影响现有 Token。扩大权限前请确认该 Token 没有泄露；过期或已撤销的 Token 不能恢复。</p>
          <div class="flex flex-wrap gap-2">
            <button class="primary-button" type="submit" :disabled="busy || !editForm.scopes.length">保存修改</button>
            <button class="soft-button" type="button" @click="editingId = ''">取消</button>
          </div>
        </form>
      </article>
    </div>
    <div class="mt-4 flex flex-wrap items-center justify-between gap-2">
      <p v-if="loadError" class="text-xs text-red-600 dark:text-red-400" role="alert">{{ loadError }}</p>
      <button v-if="tokens.length < total" class="soft-button" type="button" :disabled="loading" @click="loadTokens(false)">加载更多</button>
    </div>
  </section>
</template>

<script setup>
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import Message from '@/utils/message.js'
import { copyToClipboard } from '@/utils/clipboard.js'

defineProps({ apiEnabled: { type: Boolean, default: false } })

const scopes = ref([])
const tokens = ref([])
const total = ref(0)
const page = ref(1)
const maxActive = ref(30)
const activeCount = ref(0)
const loading = ref(false)
const loadError = ref('')
const busy = ref(false)
const issuedSecret = ref('')
const issuedTokenId = ref('')
const issuedTokenName = ref('')
const issuedAction = ref('created')
const copied = ref(false)
const editingId = ref('')
const statusLabels = { active: '有效', disabled: '已暂停', expired: '已过期', revoked: '已撤销' }
const createForm = reactive({ name: 'ScriptCat 图片上传', description: '', scopes: ['image:read', 'upload:write'], expiry: '30', customExpiry: '' })
const editForm = reactive({ name: '', description: '', scopes: [], expiresAt: '', neverExpires: false })

async function request(path, { method = 'GET', data } = {}) {
  const response = await fetch(path, {
    method,
    headers: data === undefined ? {} : { 'Content-Type': 'application/json' },
    body: data === undefined ? undefined : JSON.stringify(data),
    credentials: 'same-origin',
    cache: 'no-store'
  })
  const payload = await response.json().catch(() => ({}))
  if (!response.ok || payload.code !== 200) throw new Error(payload.message || 'API 请求失败')
  return payload.data || {}
}

const formatDate = value => {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '无效时间' : date.toLocaleString()
}

function localDatetime(value) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
}

function isoExpiry(mode, custom) {
  if (mode === 'long') return null
  const raw = mode === 'custom' ? custom : new Date(Date.now() + Number(mode) * 86400000).toISOString()
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) throw new Error('请选择有效的到期时间')
  return date.toISOString()
}

async function loadTokens(reset = true) {
  if (loading.value) return
  loading.value = true
  loadError.value = ''
  try {
    const targetPage = reset ? 1 : page.value + 1
    const data = await request(`/api/tokens?limit=50&page=${targetPage}`)
    const records = Array.isArray(data.tokens) ? data.tokens : []
    tokens.value = reset ? records : [...tokens.value, ...records]
    total.value = Number(data.total) || 0
    activeCount.value = Number(data.active_count) || 0
    maxActive.value = Number(data.max_active) || 30
    scopes.value = Array.isArray(data.available_scopes) ? data.available_scopes : []
    page.value = targetPage
  } catch (err) {
    loadError.value = err.message || '获取 Token 列表失败'
    Message.error(loadError.value)
  } finally { loading.value = false }
}

async function createToken() {
  if (busy.value || issuedSecret.value) return
  if (!createForm.scopes.length) { Message.error('至少需要选择一项权限'); return }
  let expiresAt
  try { expiresAt = isoExpiry(createForm.expiry, createForm.customExpiry) } catch (e) { Message.error(e.message); return }
  busy.value = true
  try {
    const data = await request('/api/tokens', { method: 'POST', data: {
      name: createForm.name, description: createForm.description,
      scopes: createForm.scopes, expires_at: expiresAt, never_expires: createForm.expiry === 'long'
    } })
    issuedSecret.value = data.token || data.api_token || ''
    issuedTokenId.value = data.credential?.id || ''
    issuedTokenName.value = data.credential?.name || createForm.name
    issuedAction.value = 'created'
    if (!issuedSecret.value || !issuedTokenId.value) throw new Error('服务器未返回创建后的完整 Token')
    copied.value = false
    await loadTokens(true)
    Message.success('创建成功，请复制并安全保存 Token')
  } catch (err) { Message.error(err.message || '创建失败') }
  finally { busy.value = false }
}

async function copySecret() {
  if (!issuedSecret.value) return
  if (await copyToClipboard(issuedSecret.value)) {
    copied.value = true
    Message.success('已复制到剪贴板')
  } else Message.error('复制失败，请手动选中并复制')
}

function clearSecret() {
  issuedSecret.value = ''
  issuedTokenId.value = ''
  issuedTokenName.value = ''
  issuedAction.value = 'created'
  copied.value = false
}

async function rotateToken(token) {
  if (busy.value) return
  if (issuedSecret.value) { Message.error('请先复制并隐藏当前显示的 Token，再操作其他凭据'); return }
  if (!window.confirm(`确定重新生成「${token.name}」的 Token 吗？\n\n旧密钥会立即失效，使用旧 Token 的 ScriptCat 或其他程序必须更新配置。其他 Token 不受影响。`)) return
  busy.value = true
  try {
    const data = await request(`/api/tokens/${encodeURIComponent(token.id)}/rotate`, { method: 'POST' })
    const raw = data.token || data.api_token || ''
    const newId = data.credential?.id || ''
    if (!raw || !newId) throw new Error('服务器未返回新的完整 Token')
    issuedSecret.value = raw
    issuedTokenId.value = newId
    issuedTokenName.value = data.credential?.name || token.name
    issuedAction.value = 'rotated'
    copied.value = false
    await loadTokens(true)
    Message.success('重置成功：旧 Token 已失效，新密钥之后可重复复制')
  } catch (e) { Message.error(e.message || '重置失败') }
  finally { busy.value = false }
}

// Copy uses a separate read-only management endpoint and NEVER calls /rotate.
async function copyStoredToken(token) {
  if (busy.value) return
  if (issuedSecret.value) { Message.error('请先保存并隐藏当前显示的 Token'); return }
  busy.value = true
  try {
    const data = await request(`/api/tokens/${encodeURIComponent(token.id)}/reveal`, { method: 'POST' })
    const raw = data.token || data.api_token || ''
    if (!raw) throw new Error('未能读取原 Token')
    issuedSecret.value = raw
    issuedTokenId.value = token.id
    issuedTokenName.value = token.name
    issuedAction.value = 'revealed'
    copied.value = false
    // Also show the original secret above in case the clipboard is blocked
    // by browser user-activation settings after an asynchronous request.
    await copySecret()
  } catch (err) { Message.error(err.message || '复制失败') }
  finally { busy.value = false }
}

function beginEdit(token) {
  if (editingId.value === token.id) { editingId.value = ''; return }
  editingId.value = token.id
  editForm.name = token.name
  editForm.description = token.description || ''
  editForm.scopes = [...(token.scopes || [])]
  editForm.expiresAt = localDatetime(token.expires_at)
  editForm.neverExpires = !!token.never_expires
}

async function saveEdit(token) {
  if (busy.value || !editForm.scopes.length) return
  let expiry
  try { expiry = editForm.neverExpires ? null : isoExpiry('custom', editForm.expiresAt) } catch (e) { Message.error(e.message); return }
  busy.value = true
  try {
    await request(`/api/tokens/${encodeURIComponent(token.id)}`, { method: 'PUT', data: {
      name: editForm.name, description: editForm.description,
      scopes: editForm.scopes, expires_at: expiry, never_expires: editForm.neverExpires
    } })
    editingId.value = ''
    await loadTokens(true)
    Message.success('Token 设置已保存')
  } catch (e) { Message.error(e.message || '修改失败') }
  finally { busy.value = false }
}

async function toggleToken(token) {
  if (busy.value) return
  busy.value = true
  try {
    await request(`/api/tokens/${encodeURIComponent(token.id)}/status`, { method: 'PUT', data: { disabled: !token.disabled } })
    await loadTokens(true)
    Message.success(token.disabled ? 'Token 已恢复' : 'Token 已暂停')
  } catch (e) { Message.error(e.message || '修改状态失败') }
  finally { busy.value = false }
}

async function revokeToken(token) {
  if (busy.value || !window.confirm(`永久撤销「${token.name}」？此操作不可恢复，使用该 Token 的程序会立即失效。`)) return
  busy.value = true
  try {
    await request(`/api/tokens/${encodeURIComponent(token.id)}`, { method: 'DELETE' })
    if (issuedTokenId.value === token.id) clearSecret()
    if (editingId.value === token.id) editingId.value = ''
    await loadTokens(true)
    Message.success('Token 已撤销')
  } catch (e) { Message.error(e.message || '撤销失败') }
  finally { busy.value = false }
}

onMounted(() => loadTokens(true))
onBeforeUnmount(clearSecret)
</script>
