<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { http } from '@/api/http'
import { formatDateTime } from '@/utils/format'

interface TaskRow {
  id: number
  task_id: string
  user_id: number
  user_email: string
  prompt: string
  n: number
  size: string
  upscale: string
  status: string
  result_urls_parsed: string[]
  error: string
  credit_cost: number
  estimated_credit: number
  created_at: string
  started_at?: string | null
  finished_at?: string | null
}

const loading = ref(false)
const rows = ref<TaskRow[]>([])
const total = ref(0)
const filter = reactive({
  keyword: '',
  status: '',
  range: [] as string[],
  page: 1,
  page_size: 20,
})

function withThumb(url: string, kb = 10): string {
  if (!url) return url
  const sep = url.includes('?') ? '&' : '?'
  return `${url}${sep}thumb_kb=${kb}`
}

async function fetchList() {
  loading.value = true
  try {
    const params: Record<string, any> = {
      page: filter.page,
      page_size: filter.page_size,
    }
    if (filter.keyword) params.keyword = filter.keyword
    if (filter.status) params.status = filter.status
    if (filter.range.length === 2) {
      params.start_at = filter.range[0]
      params.end_at = filter.range[1]
    }
    const data = await http.get<any, any>('/api/admin/image-tasks', { params })
    rows.value = data.list || []
    total.value = data.total || 0
  } finally {
    loading.value = false
  }
}

function onSearch() {
  filter.page = 1
  fetchList()
}

function onReset() {
  filter.keyword = ''
  filter.status = ''
  filter.range = []
  filter.page = 1
  fetchList()
}

const previewDlg = ref(false)
const previewRow = ref<TaskRow | null>(null)
const previewIdx = ref(0)
const previewUrls = computed<string[]>(() => previewRow.value?.result_urls_parsed || [])
const currentPreview = computed<string>(() => previewUrls.value[previewIdx.value] || '')

function openPreview(row: TaskRow, idx = 0) {
  previewRow.value = row
  previewIdx.value = idx
  previewDlg.value = true
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

async function downloadImage(row: TaskRow, idx: number, opts?: { silent?: boolean }): Promise<boolean> {
  const url = row.result_urls_parsed?.[idx]
  if (!url) return false
  try {
    const resp = await fetch(url, { credentials: 'include' })
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
    const blob = await resp.blob()
    const ct = blob.type || 'image/png'
    const ext = ct.includes('jpeg') ? 'jpg' : ct.split('/')[1] || 'png'
    triggerDownload(blob, `${safeName(row.prompt)}-${row.task_id}-${idx + 1}.${ext}`)
    if (!opts?.silent) {
      ElMessage.success('开始下载')
    }
    return true
  } catch (e: any) {
    if (!opts?.silent) {
      ElMessage.error(`下载失败: ${e?.message || e || 'unknown error'}`)
    }
    return false
  }
}

async function downloadAll(row: TaskRow) {
  const urls = row.result_urls_parsed || []
  if (!urls.length) return
  let ok = 0
  for (let i = 0; i < urls.length; i += 1) {
    if (await downloadImage(row, i, { silent: true })) {
      ok += 1
    }
    await new Promise((resolve) => setTimeout(resolve, 180))
  }
  if (ok > 0) {
    ElMessage.success(`已触发 ${ok} 张下载`)
  } else {
    ElMessage.error('批量下载失败')
  }
}

const statusColor: Record<string, 'success' | 'danger' | 'warning' | 'info' | 'primary'> = {
  success: 'success',
  failed: 'danger',
  running: 'warning',
  queued: 'info',
  dispatched: 'info',
}

onMounted(fetchList)
</script>

<template>
  <div class="page-container">
    <div class="card-block">
      <h2 class="page-title" style="margin:0">生成记录</h2>
      <div style="color:var(--el-text-color-secondary);font-size:13px;margin:4px 0 14px">
        全站图片生成任务历史,含用户、提示词、生成结果与耗时。
      </div>

      <el-form inline class="flex-wrap-gap" @submit.prevent="onSearch">
        <el-input v-model="filter.keyword" placeholder="提示词 / 邮箱" clearable style="width:240px" />
        <el-select v-model="filter.status" placeholder="状态" clearable style="width:130px">
          <el-option label="成功" value="success" />
          <el-option label="失败" value="failed" />
          <el-option label="运行中" value="running" />
          <el-option label="队列中" value="queued" />
          <el-option label="已分发" value="dispatched" />
        </el-select>
        <el-date-picker
          v-model="filter.range"
          type="datetimerange"
          unlink-panels
          range-separator="~"
          start-placeholder="开始时间"
          end-placeholder="结束时间"
          format="YYYY-MM-DD HH:mm"
          value-format="YYYY-MM-DD HH:mm:ss"
          style="width:340px"
        />
        <el-button type="primary" @click="onSearch"><el-icon><Search /></el-icon> 查询</el-button>
        <el-button @click="onReset">重置</el-button>
      </el-form>

      <el-table v-loading="loading" :data="rows" stripe style="margin-top:12px" size="small">
        <el-table-column prop="id" label="ID" width="72" />
        <el-table-column label="用户" min-width="170">
          <template #default="{ row }">
            <div>{{ row.user_email || '-' }}</div>
            <div style="font-size:11px;color:var(--el-text-color-secondary)">uid {{ row.user_id }}</div>
          </template>
        </el-table-column>
        <el-table-column label="提示词" min-width="240" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ row.prompt || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="规格" width="110">
          <template #default="{ row }">
            <div>{{ row.size }}</div>
            <div v-if="row.upscale" style="font-size:11px;color:var(--el-color-success)">{{ row.upscale }}</div>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="statusColor[row.status] || 'info'" size="small">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="结果" min-width="240">
          <template #default="{ row }">
            <div v-if="row.result_urls_parsed?.length" style="display:flex;align-items:center;gap:8px;flex-wrap:wrap">
              <div style="display:flex;gap:4px;flex-wrap:wrap">
                <img
                  v-for="(url, idx) in row.result_urls_parsed.slice(0, 3)"
                  :key="idx"
                  :src="withThumb(url)"
                  alt=""
                  loading="lazy"
                  style="width:44px;height:44px;border-radius:4px;object-fit:cover;cursor:zoom-in;border:1px solid var(--el-border-color-lighter)"
                  @click="openPreview(row, idx)"
                />
                <div
                  v-if="row.result_urls_parsed.length > 3"
                  style="width:44px;height:44px;border-radius:4px;display:flex;align-items:center;justify-content:center;font-size:12px;background:var(--el-fill-color-light);cursor:pointer"
                  @click="openPreview(row, 3)"
                >+{{ row.result_urls_parsed.length - 3 }}</div>
              </div>
              <div style="display:flex;flex-direction:column;gap:2px">
                <el-button type="primary" link size="small" @click="openPreview(row, 0)">放大</el-button>
                <el-button type="success" link size="small" @click="downloadImage(row, 0)">下载首张</el-button>
                <el-button
                  v-if="row.result_urls_parsed.length > 1"
                  type="warning" link size="small"
                  @click="downloadAll(row)"
                >全部下载</el-button>
              </div>
            </div>
            <span v-else-if="row.error" style="font-size:11px;color:var(--el-color-danger)" :title="row.error">失败</span>
            <span v-else style="color:var(--el-text-color-secondary)">-</span>
          </template>
        </el-table-column>
        <el-table-column label="积分" width="100">
          <template #default="{ row }">
            <div>{{ row.credit_cost }}</div>
            <div style="font-size:11px;color:var(--el-text-color-secondary)">预估 {{ row.estimated_credit }}</div>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="160">
          <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="完成时间" width="160">
          <template #default="{ row }">{{ row.finished_at ? formatDateTime(row.finished_at) : '-' }}</template>
        </el-table-column>
      </el-table>

      <el-pagination
        style="margin-top:16px;justify-content:flex-end;display:flex"
        :current-page="filter.page"
        :page-size="filter.page_size"
        :total="total"
        :page-sizes="[20, 50, 100]"
        layout="total, sizes, prev, pager, next"
        @current-change="(p: number) => { filter.page = p; fetchList() }"
        @size-change="(s: number) => { filter.page_size = s; filter.page = 1; fetchList() }"
      />
    </div>

    <el-dialog v-model="previewDlg" title="生成结果预览" width="820px">
      <div v-if="previewRow">
        <div style="font-size:13px;color:var(--el-text-color-secondary);margin-bottom:10px;word-break:break-all">
          {{ previewRow.prompt }}
        </div>
        <div style="display:flex;justify-content:center;align-items:center;background:var(--el-fill-color-darker);border-radius:6px;padding:8px;min-height:360px">
          <el-image
            :src="currentPreview"
            :preview-src-list="previewUrls"
            :initial-index="previewIdx"
            fit="contain"
            style="max-height:60vh;max-width:100%;cursor:zoom-in"
          >
            <template #placeholder>
              <div style="padding:20px;color:var(--el-text-color-secondary)">加载中…</div>
            </template>
          </el-image>
        </div>
        <div
          v-if="previewUrls.length > 1"
          style="display:flex;gap:6px;margin-top:10px;overflow-x:auto;padding-bottom:4px"
        >
          <img
            v-for="(url, idx) in previewUrls"
            :key="idx"
            :src="withThumb(url, 16)"
            alt=""
            loading="lazy"
            :class="['preview-thumb', { active: previewIdx === idx }]"
            @click="previewIdx = idx"
          />
        </div>
        <div style="display:flex;gap:8px;margin-top:12px;justify-content:flex-end">
          <el-button size="small" @click="downloadImage(previewRow, previewIdx)">下载当前</el-button>
          <el-button
            v-if="previewUrls.length > 1"
            size="small"
            type="primary"
            @click="downloadAll(previewRow)"
          >全部下载</el-button>
        </div>
        <div v-if="previewRow.error" style="margin-top:12px;color:var(--el-color-danger);font-size:12px;word-break:break-all">
          错误:{{ previewRow.error }}
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.preview-thumb {
  width: 64px;
  height: 64px;
  border-radius: 4px;
  object-fit: cover;
  cursor: pointer;
  border: 2px solid transparent;
  flex-shrink: 0;
}

.preview-thumb.active {
  border-color: var(--el-color-primary);
}
</style>
