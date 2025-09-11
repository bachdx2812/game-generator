<template>
  <div class="code-job-status">
    <!-- Not Started State -->
    <div v-if="status === 'not_started'" class="flex items-center justify-between">
      <span class="text-sm text-gray-500">Code not generated</span>
      <button @click="startCodeGeneration" :disabled="loading" class="btn-sm btn-primary">
        <svg v-if="loading" class="animate-spin h-4 w-4 mr-1" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z">
          </path>
        </svg>
        {{ loading ? 'Starting...' : 'Generate Code' }}
      </button>
    </div>

    <!-- Queued State -->
    <div v-else-if="status === 'queued'" class="flex items-center">
      <div class="flex items-center text-blue-600">
        <svg class="animate-spin h-4 w-4 mr-2" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z">
          </path>
        </svg>
        <span class="text-sm">Queued for generation</span>
      </div>
    </div>

    <!-- Processing State -->
    <div v-else-if="status === 'processing'" class="space-y-2">
      <div class="flex items-center justify-between">
        <span class="text-sm text-blue-600">Generating code...</span>
        <span class="text-xs text-gray-500">{{ progress }}%</span>
      </div>
      <div class="w-full bg-gray-200 rounded-full h-2">
        <div class="bg-blue-600 h-2 rounded-full transition-all duration-300" :style="{ width: progress + '%' }"></div>
      </div>
      <div v-if="logs && logs.length > 0" class="text-xs text-gray-600">
        {{ logs[logs.length - 1] }}
      </div>
    </div>

    <!-- Completed State -->
    <div v-else-if="status === 'completed'" class="space-y-2">
      <div class="flex items-center justify-between">
        <div class="flex items-center text-green-600">
          <svg class="h-4 w-4 mr-2" fill="currentColor" viewBox="0 0 20 20">
            <path fill-rule="evenodd"
              d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
              clip-rule="evenodd"></path>
          </svg>
          <span class="text-sm">Code generated</span>
        </div>
        <button class="btn-sm btn-outline text-xs">Download</button>
      </div>
      <div v-if="outputPath" class="text-xs text-gray-600 bg-gray-50 p-2 rounded">
        <span class="font-medium">Generated at:</span> {{ outputPath }}
      </div>
    </div>

    <!-- Failed State -->
    <div v-else-if="status === 'failed'" class="space-y-2">
      <div class="flex items-center justify-between">
        <div class="flex items-center text-red-600">
          <svg class="h-4 w-4 mr-2" fill="currentColor" viewBox="0 0 20 20">
            <path fill-rule="evenodd"
              d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z"
              clip-rule="evenodd"></path>
          </svg>
          <span class="text-sm">Generation failed</span>
        </div>
        <button @click="retryCodeGeneration" :disabled="loading"
          class="btn-sm btn-outline text-red-600 border-red-600 hover:bg-red-50">
          <svg v-if="loading" class="animate-spin h-3 w-3 mr-1" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z">
            </path>
          </svg>
          {{ loading ? 'Retrying...' : 'Retry' }}
        </button>
      </div>
      <div v-if="error" class="text-xs text-red-600 bg-red-50 p-2 rounded">
        {{ error }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'

interface Props {
  specId: string
}

const props = defineProps<Props>()

const status = ref('not_started')
const progress = ref(0)
const logs = ref<string[]>([])
const error = ref('')
const loading = ref(false)
const jobId = ref('')
const outputPath = ref('')

let pollInterval: number | null = null

const fetchCodeJobStatus = async () => {
  try {
    const response = await fetch(`/api/specs/${props.specId}/code-job`)
    if (!response.ok) throw new Error('Failed to fetch status')

    const data = await response.json()

    if (data.status === 'not_started') {
      status.value = 'not_started'
      stopPolling()
    } else {
      status.value = data.status
      progress.value = data.progress || 0
      logs.value = data.logs || []
      error.value = data.error || ''
      jobId.value = data.job_id || ''
      outputPath.value = data.output_path || ''

      // Stop polling if job is completed or failed
      if (data.status === 'completed' || data.status === 'failed') {
        stopPolling()
      }
    }
  } catch (err) {
    console.error('Error fetching code job status:', err)
  }
}

const startCodeGeneration = async () => {
  try {
    loading.value = true
    const response = await fetch('/api/code-jobs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ game_spec_id: props.specId })
    })

    if (!response.ok) throw new Error('Failed to start code generation')

    const data = await response.json()
    jobId.value = data.job_id
    status.value = 'queued'
    startPolling()
  } catch (err) {
    console.error('Error starting code generation:', err)
  } finally {
    loading.value = false
  }
}

const retryCodeGeneration = async () => {
  try {
    loading.value = true
    const response = await fetch(`/api/specs/${props.specId}/retry-code`, {
      method: 'POST'
    })

    if (!response.ok) throw new Error('Failed to retry code generation')

    const data = await response.json()
    jobId.value = data.job_id
    status.value = 'queued'
    error.value = ''
    startPolling()
  } catch (err) {
    console.error('Error retrying code generation:', err)
  } finally {
    loading.value = false
  }
}

const startPolling = () => {
  if (pollInterval) return
  pollInterval = window.setInterval(fetchCodeJobStatus, 2000)
}

const stopPolling = () => {
  if (pollInterval) {
    clearInterval(pollInterval)
    pollInterval = null
  }
}

onMounted(() => {
  fetchCodeJobStatus()
  // Start polling if job is in progress
  if (status.value === 'queued' || status.value === 'processing') {
    startPolling()
  }
})

onUnmounted(() => {
  stopPolling()
})
</script>
