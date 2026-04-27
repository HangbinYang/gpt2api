<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  getMyUsageStats,
  listMyImageTasks,
  listMyModels,
  listMyUsageLogs,
  type ImageTask,
  type MyStatsResp,
  type SimpleModel,
  type UsageItem,
} from '@/api/me'
import { ENABLE_CHAT_MODEL } from '@/config/feature'
import { formatCredit, formatDateTime, formatErrorCode } from '@/utils/format'

function withThumb(url: string, kb = 10): string {
  if (!url) return url
  const sep = url.includes('?') ? '&' : '?'
  return `${url}${sep}thumb_kb=${kb}`
}

const activeTab = ref<'chat' | 'image'>(ENABLE_CHAT_MODEL ? 'chat' : 'image')

const models = ref<SimpleModel[]>([])
const chatModels = computed(() => models.value.filter((m) => m.type === 'chat'))
const imageModels = computed(() => models.value.filter((m) => m.type === 'image'))

const selectedChatModel = ref('')
const selectedImageModel = ref('')
const origin = computed(() => window.location.origin)

const stats = ref<MyStatsResp | null>(null)
const statsLoading = ref(false)

async function loadStats() {
  statsLoading.value = true
  try {
    stats.value = await getMyUsageStats({ days: 14, top_n: 5 })
  } finally {
    statsLoading.value = false
  }
}

const chatLogs = ref<UsageItem[]>([])
const chatPage = ref({ limit: 20, offset: 0, total: 0 })
const chatLoading = ref(false)

async function loadChatLogs() {
  chatLoading.value = true
  try {
    const data = await listMyUsageLogs({
      type: 'chat',
      limit: chatPage.value.limit,
      offset: chatPage.value.offset,
    })
    chatLogs.value = data.items
    chatPage.value.total = data.total
  } finally {
    chatLoading.value = false
  }
}

function chatPageChange(page: number) {
  chatPage.value.offset = (page - 1) * chatPage.value.limit
  loadChatLogs()
}

const imageTasks = ref<ImageTask[]>([])
const imagePage = ref({ limit: 12, offset: 0 })
const imageLoading = ref(false)
const hasMoreImage = ref(false)
const imageFilter = reactive({
  status: '' as '' | 'success' | 'failed' | 'running' | 'queued' | 'dispatched',
  keyword: '',
  range: [] as string[],
})

function imageFilterParams() {
  const params: Record<string, string> = {}
  if (imageFilter.status) params.status = imageFilter.status
  if (imageFilter.keyword.trim()) params.keyword = imageFilter.keyword.trim()
  if (imageFilter.range.length === 2) {
    params.start_at = imageFilter.range[0]
    params.end_at = imageFilter.range[1]
  }
  return params
}

async function loadImageTasks(reset = true) {
  imageLoading.value = true
  try {
    if (reset) {
      imagePage.value.offset = 0
      imageTasks.value = []
    }
    const data = await listMyImageTasks({
      limit: imagePage.value.limit,
      offset: imagePage.value.offset,
      ...imageFilterParams(),
    })
    if (reset) imageTasks.value = data.items
    else imageTasks.value.push(...data.items)
    hasMoreImage.value = data.items.length >= imagePage.value.limit
  } finally {
    imageLoading.value = false
  }
}

function imageLoadMore() {
  imagePage.value.offset += imagePage.value.limit
  loadImageTasks(false)
}

function onImageFilterReset() {
  imageFilter.status = ''
  imageFilter.keyword = ''
  imageFilter.range = []
  loadImageTasks(true)
}

const imgPreviewDlg = ref(false)
const imgPreviewTask = ref<ImageTask | null>(null)
const imgPreviewIdx = ref(0)
const imgPreviewUrls = computed<string[]>(() => imgPreviewTask.value?.image_urls || [])
const imgPreviewCurrent = computed<string>(() => imgPreviewUrls.value[imgPreviewIdx.value] || '')

function openImagePreview(task: ImageTask, idx = 0) {
  if (!task.image_urls?.length) return
  imgPreviewTask.value = task
  imgPreviewIdx.value = idx
  imgPreviewDlg.value = true
}

function safeName(s: string) {
  return (s || 'image').replace(/[\\/:*?"<>|]/g, '_').slice(0, 24) || 'image'
}

function triggerDownload(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  setTimeout(() => URL.revokeObjectURL(url), 60_000)
}

async function downloadImageOne(task: ImageTask, idx: number) {
  const url = task.image_urls?.[idx]
  if (!url) return
  try {
    const resp = await fetch(url, { credentials: 'include' })
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
    const blob = await resp.blob()
    const ct = blob.type || 'image/png'
    const ext = ct.includes('jpeg') ? 'jpg' : ct.split('/')[1] || 'png'
    triggerDownload(blob, `${safeName(task.prompt)}-${task.task_id}-${idx + 1}.${ext}`)
  } catch (e: any) {
    ElMessage.error(`下载失败: ${e?.message || e || 'unknown error'}`)
  }
}

async function downloadImageAll(task: ImageTask) {
  const urls = task.image_urls || []
  if (!urls.length) return
  let ok = 0
  for (let i = 0; i < urls.length; i += 1) {
    try {
      await downloadImageOne(task, i)
      ok += 1
      await new Promise((resolve) => setTimeout(resolve, 180))
    } catch {
      // 单张失败不阻断批量下载
    }
  }
  if (ok > 1) {
    ElMessage.success(`已触发 ${ok} 张下载`)
  }
}

const chatCurl = computed(() => {
  const model = selectedChatModel.value || 'gpt-5'
  return `curl ${origin.value}/v1/chat/completions \\
  -H "Authorization: Bearer \${YOUR_API_KEY}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${model}",
    "stream": true,
    "messages": [
      {"role": "user", "content": "你好,介绍一下你自己"}
    ]
  }'`
})

const chatPython = computed(() => {
  const model = selectedChatModel.value || 'gpt-5'
  return `from openai import OpenAI

client = OpenAI(
    base_url="${origin.value}/v1",
    api_key="\${YOUR_API_KEY}",
)

resp = client.chat.completions.create(
    model="${model}",
    messages=[{"role": "user", "content": "你好"}],
    stream=True,
)
for chunk in resp:
    print(chunk.choices[0].delta.content or "", end="")`
})

const imageCurl = computed(() => {
  const model = selectedImageModel.value || 'gpt-image-2'
  return `curl ${origin.value}/v1/images/generations \\
  -H "Authorization: Bearer \${YOUR_API_KEY}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${model}",
    "prompt": "A cute orange cat playing with yarn, studio ghibli style",
    "n": 1,
    "size": "1024x1024"
  }'`
})

const imageRefCurl = computed(() => {
  const model = selectedImageModel.value || 'gpt-image-2'
  return `curl ${origin.value}/v1/images/generations \\
  -H "Authorization: Bearer \${YOUR_API_KEY}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${model}",
    "prompt": "Restyle the cat as a watercolor painting, soft pastel palette",
    "n": 1,
    "size": "1024x1024",
    "reference_images": [
      "https://example.com/cat.png",
      "data:image/png;base64,iVBORw0KGgo..."
    ]
  }'`
})

const imagePython = computed(() => {
  const model = selectedImageModel.value || 'gpt-image-2'
  return `from openai import OpenAI

client = OpenAI(
    base_url="${origin.value}/v1",
    api_key="\${YOUR_API_KEY}",
)

resp = client.images.generate(
    model="${model}",
    prompt="A cute orange cat playing with yarn",
    n=1,
    size="1024x1024",
)
print(resp.data[0].url)`
})

const imagePythonRequests = computed(() => {
  const model = selectedImageModel.value || 'gpt-image-2'
  return `import requests

API_KEY = "\${YOUR_API_KEY}"
BASE_URL = "${origin.value}/v1"

resp = requests.post(
    f"{BASE_URL}/images/generations",
    headers={
        "Authorization": f"Bearer {API_KEY}",
        "Content-Type": "application/json",
    },
    json={
        "model": "${model}",
        "prompt": "A cute orange cat playing with yarn",
        "n": 1,
        "size": "1024x1024",
    },
    timeout=300,
)
resp.raise_for_status()
print(resp.json()["data"][0]["url"])`
})

const imagePythonRefRequests = computed(() => {
  const model = selectedImageModel.value || 'gpt-image-2'
  return `import base64, requests

API_KEY = "\${YOUR_API_KEY}"
BASE_URL = "${origin.value}/v1"

def img_b64(path: str) -> str:
    with open(path, "rb") as f:
        return base64.b64encode(f.read()).decode()

resp = requests.post(
    f"{BASE_URL}/images/generations",
    headers={
        "Authorization": f"Bearer {API_KEY}",
        "Content-Type": "application/json",
    },
    json={
        "model": "${model}",
        "prompt": "Turn the cat into a watercolor painting",
        "n": 1,
        "size": "1024x1024",
        "reference_images": [
            img_b64("cat.png"),
            # 也可以直接传公网 URL:"https://example.com/style.jpg"
        ],
    },
    timeout=300,
)
resp.raise_for_status()
print(resp.json()["data"][0]["url"])`
})

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败,请手动选择文本')
  }
}

function statusTag(s: string): 'success' | 'warning' | 'danger' | 'info' {
  if (s === 'success') return 'success'
  if (s === 'failed') return 'danger'
  if (s === 'running' || s === 'dispatched' || s === 'queued') return 'warning'
  return 'info'
}

onMounted(async () => {
  try {
    const m = await listMyModels()
    models.value = ENABLE_CHAT_MODEL ? m.items : m.items.filter((x) => x.type !== 'chat')
    const firstChat = m.items.find((x) => x.type === 'chat')
    const firstImage = m.items.find((x) => x.type === 'image')
    if (firstChat) selectedChatModel.value = firstChat.slug
    if (firstImage) selectedImageModel.value = firstImage.slug
  } catch {
    // ignore
  }
  loadStats()
  if (ENABLE_CHAT_MODEL) loadChatLogs()
  loadImageTasks(true)
})
</script>

<template>
  <div class="page-container">
    <div class="card-block hero">
      <div>
        <h2 class="page-title">接口文档 & 用量</h2>
        <p class="desc">
          <template v-if="ENABLE_CHAT_MODEL">
            外部调用走 <code>/v1/chat/completions</code> 与 <code>/v1/images/generations</code>,
          </template>
          <template v-else>
            外部调用走 <code>/v1/images/generations</code>,
          </template>
          下面给出 curl / Python 代码片段;个人用量与图片任务汇总也在这里。若想在浏览器里直接体验,请打开「在线体验」。
        </p>
      </div>
      <div class="hero-stats" v-loading="statsLoading">
        <div class="stat">
          <div class="lbl">14 天请求</div>
          <div class="val">{{ stats?.overall.requests ?? 0 }}</div>
        </div>
        <div v-if="ENABLE_CHAT_MODEL" class="stat">
          <div class="lbl">文字 Token(in/out)</div>
          <div class="val">{{ stats?.overall.input_tokens ?? 0 }} / {{ stats?.overall.output_tokens ?? 0 }}</div>
        </div>
        <div class="stat">
          <div class="lbl">图片张数</div>
          <div class="val">{{ stats?.overall.image_images ?? 0 }}</div>
        </div>
        <div class="stat">
          <div class="lbl">14 天消耗积分</div>
          <div class="val primary">{{ formatCredit(stats?.overall.credit_cost ?? 0) }}</div>
        </div>
      </div>
    </div>

    <el-tabs v-model="activeTab" class="pg-tabs">
      <el-tab-pane v-if="ENABLE_CHAT_MODEL" label="对话生成(文字模型)" name="chat">
        <div class="card-block">
          <div class="row">
            <div class="label">文字模型</div>
            <el-select v-model="selectedChatModel" placeholder="选择模型" style="width: 320px">
              <el-option
                v-for="m in chatModels"
                :key="m.id"
                :label="`${m.slug}${m.description ? ' · ' + m.description : ''}`"
                :value="m.slug"
              />
            </el-select>
            <router-link to="/personal/keys">
              <el-button text type="primary">没有 Key?去「API Keys」创建</el-button>
            </router-link>
          </div>

          <el-tabs type="border-card" class="code-tabs">
            <el-tab-pane label="curl">
              <pre class="code"><code>{{ chatCurl }}</code></pre>
              <el-button size="small" @click="copy(chatCurl)">复制 curl</el-button>
            </el-tab-pane>
            <el-tab-pane label="Python (OpenAI SDK)">
              <pre class="code"><code>{{ chatPython }}</code></pre>
              <el-button size="small" @click="copy(chatPython)">复制 Python</el-button>
            </el-tab-pane>
          </el-tabs>
        </div>

        <div class="card-block">
          <div class="flex-between" style="margin-bottom: 10px">
            <h3 class="section-title">文字调用历史</h3>
            <el-button size="small" @click="loadChatLogs">刷新</el-button>
          </div>
          <el-table v-loading="chatLoading" :data="chatLogs" stripe size="small">
            <el-table-column prop="created_at" label="时间" min-width="160">
              <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
            </el-table-column>
            <el-table-column prop="model_slug" label="模型" min-width="140" />
            <el-table-column label="Token (in / out / cache)" min-width="170">
              <template #default="{ row }">
                {{ row.input_tokens }} / {{ row.output_tokens }}
                <span v-if="row.cache_read_tokens" class="mute">/ {{ row.cache_read_tokens }}</span>
              </template>
            </el-table-column>
            <el-table-column label="耗时" width="90">
              <template #default="{ row }">{{ row.duration_ms }} ms</template>
            </el-table-column>
            <el-table-column label="状态" width="90">
              <template #default="{ row }">
                <el-tag :type="statusTag(row.status)" size="small">{{ row.status }}</el-tag>
                <el-tooltip v-if="row.error_code" :content="formatErrorCode(row.error_code) + '(' + row.error_code + ')'">
                  <el-icon style="margin-left:4px"><InfoFilled /></el-icon>
                </el-tooltip>
              </template>
            </el-table-column>
            <el-table-column label="扣费(积分)" width="110">
              <template #default="{ row }">{{ formatCredit(row.credit_cost) }}</template>
            </el-table-column>
          </el-table>
          <div class="pager">
            <el-pagination
              layout="prev, pager, next, total"
              :total="chatPage.total"
              :page-size="chatPage.limit"
              :current-page="Math.floor(chatPage.offset / chatPage.limit) + 1"
              @current-change="chatPageChange"
            />
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane label="图片生成(图片模型)" name="image">
        <div class="card-block">
          <div class="row">
            <div class="label">图片模型</div>
            <el-select v-model="selectedImageModel" placeholder="选择模型" style="width: 320px">
              <el-option
                v-for="m in imageModels"
                :key="m.id"
                :label="`${m.slug}${m.description ? ' · ' + m.description : ''}`"
                :value="m.slug"
              />
            </el-select>
          </div>

          <el-tabs type="border-card" class="code-tabs">
            <el-tab-pane label="curl(文生图)">
              <pre class="code"><code>{{ imageCurl }}</code></pre>
              <el-button size="small" @click="copy(imageCurl)">复制 curl</el-button>
            </el-tab-pane>
            <el-tab-pane label="curl(图生图)">
              <pre class="code"><code>{{ imageRefCurl }}</code></pre>
              <div class="hint">
                reference_images 支持 <code>URL</code> / <code>data:URL</code> / 纯 <code>base64</code>;
                单次最多 4 张,单张最大 20MB。
              </div>
              <el-button size="small" @click="copy(imageRefCurl)">复制 curl</el-button>
            </el-tab-pane>
            <el-tab-pane label="Python (OpenAI SDK)">
              <pre class="code"><code>{{ imagePython }}</code></pre>
              <el-button size="small" @click="copy(imagePython)">复制 Python</el-button>
            </el-tab-pane>
            <el-tab-pane label="Python (requests · 文生图)">
              <pre class="code"><code>{{ imagePythonRequests }}</code></pre>
              <el-button size="small" @click="copy(imagePythonRequests)">复制 Python</el-button>
            </el-tab-pane>
            <el-tab-pane label="Python (requests · 图生图)">
              <pre class="code"><code>{{ imagePythonRefRequests }}</code></pre>
              <div class="hint">
                reference_images 同时支持 <code>URL</code> / <code>data:URL</code> / 纯 <code>base64</code>,
                最多 4 张、单张最大 20MB,服务端会自动下载并解码。
              </div>
              <el-button size="small" @click="copy(imagePythonRefRequests)">复制 Python</el-button>
            </el-tab-pane>
          </el-tabs>
        </div>

        <div class="card-block">
          <div class="flex-between" style="margin-bottom: 10px">
            <h3 class="section-title">图片任务历史</h3>
            <el-button size="small" @click="loadImageTasks(true)">刷新</el-button>
          </div>
          <el-form inline class="flex-wrap-gap" style="margin-bottom:10px" @submit.prevent="loadImageTasks(true)">
            <el-input v-model="imageFilter.keyword" placeholder="提示词关键字" clearable style="width:220px" />
            <el-select v-model="imageFilter.status" placeholder="状态" clearable style="width:130px">
              <el-option label="成功" value="success" />
              <el-option label="失败" value="failed" />
              <el-option label="运行中" value="running" />
              <el-option label="队列中" value="queued" />
              <el-option label="已分发" value="dispatched" />
            </el-select>
            <el-date-picker
              v-model="imageFilter.range"
              type="datetimerange"
              unlink-panels
              range-separator="~"
              start-placeholder="开始时间"
              end-placeholder="结束时间"
              format="YYYY-MM-DD HH:mm"
              value-format="YYYY-MM-DD HH:mm:ss"
              style="width:340px"
            />
            <el-button type="primary" @click="loadImageTasks(true)">查询</el-button>
            <el-button @click="onImageFilterReset">重置</el-button>
          </el-form>

          <div v-loading="imageLoading">
            <div v-if="imageTasks.length === 0 && !imageLoading" class="empty">
              暂无图片任务,复制上方代码调用一次即可生成记录。
            </div>
            <div class="grid">
              <el-card
                v-for="task in imageTasks"
                :key="task.id"
                shadow="hover"
                class="img-card"
              >
                <div class="thumb" @click="openImagePreview(task, 0)">
                  <img
                    v-if="task.image_urls?.[0]"
                    :src="withThumb(task.image_urls[0])"
                    :alt="task.prompt"
                    loading="lazy"
                  />
                  <div v-else class="thumb-ph">
                    <el-icon :size="32"><PictureRounded /></el-icon>
                    <div class="s">{{ task.status }}</div>
                  </div>
                  <div v-if="task.image_urls?.length > 1" class="thumb-badge">
                    {{ task.image_urls.length }} 张
                  </div>
                </div>
                <div class="meta">
                  <div class="title" :title="task.prompt">{{ task.prompt || '(无 prompt)' }}</div>
                  <div class="sub">
                    <el-tag size="small" :type="statusTag(task.status)">{{ task.status }}</el-tag>
                    <span>{{ task.size }}</span>
                    <span class="mute">n={{ task.n }}</span>
                    <span v-if="task.upscale" class="upscale">{{ task.upscale }}</span>
                  </div>
                  <div class="foot">
                    <span class="mute">{{ formatDateTime(task.created_at) }}</span>
                    <span class="credit">{{ formatCredit(task.credit_cost) }} 积分</span>
                  </div>
                  <div class="actions">
                    <el-button
                      v-if="task.image_urls?.length"
                      size="small"
                      type="primary"
                      link
                      @click="openImagePreview(task, 0)"
                    >放大</el-button>
                    <el-button
                      v-if="task.image_urls?.length"
                      size="small"
                      link
                      @click="downloadImageOne(task, 0)"
                    >下载首张</el-button>
                    <el-button
                      v-if="task.image_urls?.length > 1"
                      size="small"
                      type="warning"
                      link
                      @click="downloadImageAll(task)"
                    >全部下载</el-button>
                  </div>
                  <div v-if="task.error" class="err">{{ task.error }}</div>
                </div>
              </el-card>
            </div>
            <div v-if="hasMoreImage" class="pager">
              <el-button @click="imageLoadMore">加载更多</el-button>
            </div>
          </div>
        </div>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="imgPreviewDlg" title="图片预览" width="780px">
      <div v-if="imgPreviewTask">
        <div class="prompt-line" :title="imgPreviewTask.prompt">{{ imgPreviewTask.prompt }}</div>
        <div class="big-img-wrap">
          <el-image
            :src="imgPreviewCurrent"
            :preview-src-list="imgPreviewUrls"
            :initial-index="imgPreviewIdx"
            fit="contain"
            style="max-height:60vh;max-width:100%;cursor:zoom-in"
          />
        </div>
        <div v-if="imgPreviewUrls.length > 1" class="thumb-strip">
          <img
            v-for="(url, idx) in imgPreviewUrls"
            :key="idx"
            :src="withThumb(url, 16)"
            alt=""
            loading="lazy"
            :class="['p-thumb', { active: idx === imgPreviewIdx }]"
            @click="imgPreviewIdx = idx"
          />
        </div>
        <div class="dlg-actions">
          <el-button size="small" @click="downloadImageOne(imgPreviewTask, imgPreviewIdx)">下载当前</el-button>
          <el-button
            v-if="imgPreviewUrls.length > 1"
            size="small"
            type="primary"
            @click="downloadImageAll(imgPreviewTask)"
          >全部下载</el-button>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.page-container { padding: 16px; }
.page-title { margin: 0; font-size: 20px; font-weight: 700; }
.section-title { margin: 0; font-size: 16px; font-weight: 600; }

.card-block {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  padding: 16px;
  margin-bottom: 16px;
}

.flex-between { display: flex; justify-content: space-between; align-items: center; }

.hero {
  display: flex;
  justify-content: space-between;
  gap: 24px;
  flex-wrap: wrap;

  .desc { color: var(--el-text-color-secondary); margin-top: 4px; font-size: 13px; }

  code {
    background: var(--el-fill-color-light);
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 12px;
  }
}

.hero-stats {
  display: flex;
  gap: 24px;
  flex-wrap: wrap;

  .stat { min-width: 120px; }
  .lbl { font-size: 12px; color: var(--el-text-color-secondary); }
  .val { font-size: 22px; font-weight: 700; margin-top: 2px; }
  .val.primary { color: #409eff; }
}

.pg-tabs { :deep(.el-tabs__header) { margin-bottom: 12px; } }

.row {
  display: flex;
  gap: 12px;
  align-items: center;
  flex-wrap: wrap;
  margin-bottom: 12px;

  .label { font-weight: 600; min-width: 68px; }
}

.code-tabs {
  :deep(.el-tabs__content) { padding: 12px; }
}

.code {
  background: #1f2937;
  color: #e5e7eb;
  border-radius: 6px;
  padding: 12px 14px;
  margin: 0 0 10px;
  font-size: 12px;
  line-height: 1.6;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
}

:global(html.dark) .code { background: #0f1115; }

.hint {
  margin-bottom: 10px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.6;

  code {
    background: var(--el-fill-color-light);
    padding: 1px 4px;
    border-radius: 4px;
  }
}

.mute { color: var(--el-text-color-secondary); }
.pager { margin-top: 12px; display: flex; justify-content: flex-end; }
.empty { padding: 24px 0; color: var(--el-text-color-secondary); text-align: center; }

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 14px;
}

.img-card {
  :deep(.el-card__body) {
    padding: 12px;
  }
}

.thumb {
  position: relative;
  height: 180px;
  border-radius: 10px;
  overflow: hidden;
  background: var(--el-fill-color-light);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: zoom-in;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }
}

.thumb-ph {
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: center;
  justify-content: center;
  color: var(--el-text-color-secondary);

  .s { font-size: 12px; }
}

.thumb-badge {
  position: absolute;
  right: 8px;
  top: 8px;
  background: rgba(0, 0, 0, 0.6);
  color: #fff;
  border-radius: 999px;
  font-size: 12px;
  padding: 2px 8px;
}

.meta {
  margin-top: 12px;

  .title {
    font-weight: 600;
    line-height: 1.5;
    min-height: 42px;
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    overflow: hidden;
  }

  .sub,
  .foot,
  .actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    margin-top: 8px;
    font-size: 12px;
  }

  .upscale {
    color: var(--el-color-success);
    font-weight: 600;
  }

  .credit {
    margin-left: auto;
    font-weight: 600;
  }

  .err {
    margin-top: 8px;
    color: var(--el-color-danger);
    font-size: 12px;
    word-break: break-all;
  }
}

.prompt-line {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  margin-bottom: 10px;
  word-break: break-all;
}

.big-img-wrap {
  display: flex;
  justify-content: center;
  align-items: center;
  background: var(--el-fill-color-darker);
  border-radius: 6px;
  padding: 8px;
  min-height: 360px;
}

.thumb-strip {
  display: flex;
  gap: 6px;
  margin-top: 10px;
  overflow-x: auto;
  padding-bottom: 4px;
}

.p-thumb {
  width: 64px;
  height: 64px;
  border-radius: 4px;
  object-fit: cover;
  cursor: pointer;
  border: 2px solid transparent;
  flex-shrink: 0;
}

.p-thumb.active {
  border-color: var(--el-color-primary);
}

.dlg-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 12px;
}
</style>
