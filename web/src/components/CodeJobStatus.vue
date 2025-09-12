<template>
  <div class="code-job-status">
    <!-- Creating State -->
    <div v-if="gameState === 'creating'" class="flex items-center">
      <div class="flex items-center text-blue-600">
        <svg class="animate-spin h-4 w-4 mr-2" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z">
          </path>
        </svg>
        <span class="text-sm">Creating specification...</span>
      </div>
    </div>

    <!-- Git Initializing State -->
    <div v-else-if="gameState === 'git_initing'" class="flex items-center">
      <div class="flex items-center text-yellow-600">
        <svg class="animate-spin h-4 w-4 mr-2" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z">
          </path>
        </svg>
        <span class="text-sm">Initializing Git repository...</span>
      </div>
    </div>

    <!-- Git Ready State -->
    <div v-else-if="gameState === 'git_inited'" class="space-y-2">
      <div class="flex items-center justify-between">
        <div class="flex items-center text-indigo-600">
          <svg class="h-4 w-4 mr-2" fill="currentColor" viewBox="0 0 20 20">
            <path fill-rule="evenodd"
              d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
              clip-rule="evenodd"></path>
          </svg>
          <span class="text-sm">Git repository ready</span>
        </div>
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
      <div v-if="gitRepoUrl" class="text-xs text-gray-600 bg-gray-50 p-2 rounded">
        <span class="font-medium">Repository:</span>
        <a :href="gitRepoUrl" target="_blank" class="text-blue-600 hover:underline">{{ gitRepoUrl }}</a>
      </div>
    </div>

    <!-- Code Generating State -->
    <div v-else-if="gameState === 'code_generating'" class="space-y-2">
      <div class="flex items-center justify-between">
        <span class="text-sm text-purple-600">Generating code...</span>
        <span class="text-xs text-gray-500">{{ progress }}%</span>
      </div>
      <div class="w-full bg-gray-200 rounded-full h-2">
        <div class="bg-purple-600 h-2 rounded-full transition-all duration-300" :style="{ width: progress + '%' }"></div>
      </div>
      <div v-if="logs && logs.length > 0" class="text-xs text-gray-600">
        {{ logs[logs.length - 1] }}
      </div>
    </div>

    <!-- Code Generated State -->
    <div v-else-if="gameState === 'code_generated'" class="space-y-2">
      <div class="flex items-center justify-between">
        <div class="flex items-center text-green-600">
          <svg class="h-4 w-4 mr-2" fill="currentColor" viewBox="0 0 20 20">
            <path fill-rule="evenodd"
              d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
              clip-rule="evenodd"></path>
          </svg>
          <span class="text-sm">Code generated successfully</span>
        </div>
        <a v-if="gitRepoUrl" :href="gitRepoUrl" target="_blank" class="btn-sm btn-outline text-xs">
          View on GitHub
        </a>
      </div>
      <div v-if="gitRepoUrl" class="text-xs text-gray-600 bg-gray-50 p-2 rounded">
        <span class="font-medium">Repository:</span>
        <a :href="gitRepoUrl" target="_blank" class="text-blue-600 hover:underline">{{ gitRepoUrl }}</a>
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

const gameState = ref('creating')
const progress = ref(0)
const logs = ref<string[]>([])
const error = ref('')
const loading = ref(false)
const jobId = ref('')
const gitRepoUrl = ref('')

let pollInterval: number | null = null

const fetchGameSpecStatus = async () => {
  try {
    const response = await fetch(`/api/specs/${props.specId}`)
    if (!response.ok) throw new Error('Failed to fetch spec status')

    const data = await response.json()
    gameState.value = data.state || 'creating'
    gitRepoUrl.value = data.git_repo_url || ''

    // If code is being generated, also fetch code job status
    if (gameState.value === 'code_generating') {
      await fetchCodeJobStatus()
    }

    // Stop polling if code is generated
    if (gameState.value === 'code_generated') {
      stopPolling()
    }
  } catch (err) {
    console.error('Error fetching game spec status:', err)
  }
}

const fetchCodeJobStatus = async () => {
  try {
    const response = await fetch(`/api/specs/${props.specId}/code-job`)
    if (!response.ok) return

    const data = await response.json()
    progress.value = data.progress || 0
    logs.value = data.logs || []
    error.value = data.error || ''
    jobId.value = data.job_id || ''
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
    gameState.value = 'code_generating'
    startPolling()
  } catch (err) {
    console.error('Error starting code generation:', err)
  } finally {
    loading.value = false
  }
}

const startPolling = () => {
  if (pollInterval) return
  pollInterval = window.setInterval(fetchGameSpecStatus, 2000)
}

const stopPolling = () => {
  if (pollInterval) {
    clearInterval(pollInterval)
    pollInterval = null
  }
}

onMounted(() => {
  fetchGameSpecStatus()
  // Start polling if game is in progress
  if (['creating', 'git_initing', 'code_generating'].includes(gameState.value)) {
    startPolling()
  }
})

onUnmounted(() => {
  stopPolling()
})
</script>
