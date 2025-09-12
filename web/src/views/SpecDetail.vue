<template>
  <div>
    <!-- Loading State -->
    <div v-if="loading" class="flex justify-center py-12">
      <svg class="animate-spin h-8 w-8 text-primary-500" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor"
          d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z">
        </path>
      </svg>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded-lg">
      {{ error }}
    </div>

    <!-- Spec Detail -->
    <div v-else-if="spec">
      <!-- Header -->
      <div class="flex items-center justify-between mb-8">
        <div>
          <button @click="$router.back()"
            class="flex items-center text-gray-600 hover:text-gray-900 mb-4 transition-colors">
            <svg class="w-5 h-5 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"></path>
            </svg>
            Back
          </button>
          <h1 class="text-3xl font-bold text-gray-900">{{ spec.title }}</h1>
          <p class="text-gray-600 mt-1">{{ spec.brief }}</p>
          <!-- Add state display -->
          <div class="mt-3">
            <span :class="getStateClass(spec.state)"
              class="inline-flex items-center px-3 py-1 rounded-full text-sm font-medium">
              <span :class="getStateDotClass(spec.state)" class="w-2 h-2 rounded-full mr-2"></span>
              {{ getStateLabel(spec.state) }}
            </span>
          </div>
        </div>
        <div class="flex space-x-3">
          <button @click="copyToClipboard(JSON.stringify(spec.spec_json, null, 2))" class="btn-secondary">
            Copy JSON
          </button>
          <button @click="runDevinTask" :disabled="devinTaskLoading"
            class="px-4 py-2 text-sm font-medium text-white bg-green-600 border border-transparent rounded-md hover:bg-green-700 disabled:bg-gray-400 disabled:cursor-not-allowed focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500 transition-colors duration-200">
            <svg v-if="!devinTaskLoading" class="w-4 h-4 mr-2 inline" fill="none" stroke="currentColor"
              viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z">
              </path>
            </svg>
            <svg v-else class="animate-spin w-4 h-4 mr-2 inline" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z">
              </path>
            </svg>
            {{ devinTaskLoading ? 'Creating Task...' : 'Run Devin Task' }}
          </button>
          <button @click="showDeleteDialog = true"
            class="px-4 py-2 text-sm font-medium text-white bg-red-600 border border-transparent rounded-md hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500">
            Delete Spec
          </button>
          <router-link to="/" class="btn-primary">
            Generate New
          </router-link>
        </div>
      </div>

      <!-- Game Overview -->
      <div class="grid lg:grid-cols-3 gap-8 mb-8">
        <div class="lg:col-span-2">
          <div class="card">
            <h2 class="text-xl font-semibold text-gray-900 mb-4">Game Overview</h2>
            <div class="grid md:grid-cols-2 gap-6">
              <div>
                <h3 class="font-medium text-gray-900 mb-2">Basic Info</h3>
                <div class="space-y-2 text-sm">
                  <div><span class="font-medium text-gray-600">Genre:</span> {{ spec.spec_json?.genre || 'N/A' }}</div>
                  <div><span class="font-medium text-gray-600">Duration:</span> {{ spec.spec_json?.duration_sec || 'N/A'
                    }} seconds</div>
                  <div><span class="font-medium text-gray-600">Platform:</span> {{ spec.spec_json?.platform?.join(', ')
                    || 'N/A' }}</div>
                  <div><span class="font-medium text-gray-600">Controls:</span> {{ spec.spec_json?.controls?.join(', ')
                    || 'N/A' }}</div>
                </div>
              </div>
              <div>
                <h3 class="font-medium text-gray-900 mb-2">Game Modes</h3>
                <div class="space-y-2">
                  <div v-for="mode in spec.spec_json?.game_modes || []" :key="mode.mode" class="text-sm">
                    <div class="flex items-center mb-1">
                      <span class="w-2 h-2 bg-primary-500 rounded-full mr-2"></span>
                      <span class="font-medium">{{ mode.mode?.replace('_', ' ').toUpperCase() || 'Unknown' }}</span>
                    </div>
                    <p class="text-gray-600 ml-4">{{ mode.description || 'No description' }}</p>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div>
          <div class="card">
            <h2 class="text-xl font-semibold text-gray-900 mb-4">Quick Stats</h2>
            <div class="space-y-3">
              <div class="flex justify-between">
                <span class="text-gray-600">Mechanics</span>
                <span class="font-medium">{{ spec.spec_json?.detailed_mechanics?.length || 0 }}</span>
              </div>
              <div class="flex justify-between">
                <span class="text-gray-600">Game Modes</span>
                <span class="font-medium">{{ spec.spec_json?.game_modes?.length || 0 }}</span>
              </div>
              <div class="flex justify-between">
                <span class="text-gray-600">Assets</span>
                <span class="font-medium">{{ spec.spec_json?.assets?.length || 0 }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Devin Session Info -->
      <div v-if="spec.devin_session_id" class="card">
        <h2 class="text-xl font-semibold text-gray-900 mb-4">Devin Session</h2>
        <div class="bg-green-50 border border-green-200 rounded-lg p-4">
          <div class="flex items-center justify-between">
            <div>
              <p class="text-sm font-medium text-green-800 mb-1">Active Devin Session</p>
              <p class="text-sm text-green-700">Session ID: {{ spec.devin_session_id }}</p>
            </div>
            <a :href="spec.devin_session_url" target="_blank" rel="noopener noreferrer"
              class="inline-flex items-center px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 transition-colors duration-200 font-medium text-sm">
              <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                  d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"></path>
              </svg>
              Open in Devin
            </a>
          </div>
        </div>
      </div>

      <!-- Detailed Sections -->
      <div class="space-y-8">
        <!-- Game Mechanics -->
        <div v-if="spec.spec_json?.detailed_mechanics?.length" class="card">
          <h2 class="text-xl font-semibold text-gray-900 mb-4">Game Mechanics</h2>
          <div class="grid md:grid-cols-2 gap-6">
            <div v-for="mechanic in spec.spec_json.detailed_mechanics" :key="mechanic.mechanic_name"
              class="border-l-4 border-primary-500 pl-4">
              <h3 class="font-medium text-gray-900 mb-2">{{ mechanic.mechanic_name }}</h3>
              <p class="text-gray-700 text-sm mb-2">{{ mechanic.description }}</p>
              <p class="text-gray-600 text-xs"><span class="font-medium">Interaction:</span> {{
                mechanic.player_interaction }}</p>
            </div>
          </div>
        </div>

        <!-- Technical Requirements -->
        <div v-if="spec.spec_json?.technical_requirements" class="card">
          <h2 class="text-xl font-semibold text-gray-900 mb-4">Technical Requirements</h2>
          <div class="grid md:grid-cols-2 gap-6">
            <div v-for="(value, key) in spec.spec_json.technical_requirements" :key="key">
              <h3 class="font-medium text-gray-900 mb-2">{{ String(key).replace('_', ' ').replace(/\b\w/g, l =>
                l.toUpperCase()) }}</h3>
              <p class="text-gray-700 text-sm">{{ Array.isArray(value) ? value.join(', ') : value }}</p>
            </div>
          </div>
        </div>

        <!-- Raw Data Tabs -->
        <div class="card">
          <div class="border-b border-gray-200 mb-6">
            <nav class="-mb-px flex space-x-8">
              <button @click="activeTab = 'markdown'"
                :class="activeTab === 'markdown' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'"
                class="py-2 px-1 border-b-2 font-medium text-sm transition-colors">
                Markdown
              </button>
              <button @click="activeTab = 'json'"
                :class="activeTab === 'json' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'"
                class="py-2 px-1 border-b-2 font-medium text-sm transition-colors">
                JSON
              </button>
            </nav>
          </div>

          <div v-if="activeTab === 'markdown'">
            <div class="prose prose-lg max-w-none text-gray-800 bg-white p-6 rounded-lg border shadow-sm overflow-auto max-h-[600px]
                        prose-headings:text-gray-900 prose-headings:font-semibold
                        prose-h1:text-2xl prose-h1:mb-4 prose-h1:mt-6
                        prose-h2:text-xl prose-h2:mb-3 prose-h2:mt-5 prose-h2:border-b prose-h2:border-gray-200 prose-h2:pb-2
                        prose-h3:text-lg prose-h3:mb-2 prose-h3:mt-4
                        prose-p:mb-4 prose-p:leading-relaxed
                        prose-ul:mb-4 prose-ul:pl-6
                        prose-ol:mb-4 prose-ol:pl-6
                        prose-li:mb-1
                        prose-strong:text-gray-900 prose-strong:font-semibold
                        prose-em:text-gray-700
                        prose-code:bg-gray-100 prose-code:px-1 prose-code:py-0.5 prose-code:rounded prose-code:text-sm prose-code:font-mono
                        prose-pre:bg-gray-900 prose-pre:text-gray-100 prose-pre:p-4 prose-pre:rounded-lg prose-pre:overflow-x-auto
                        prose-blockquote:border-l-4 prose-blockquote:border-blue-500 prose-blockquote:pl-4 prose-blockquote:italic prose-blockquote:text-gray-600
                        prose-table:border-collapse prose-table:w-full
                        prose-th:border prose-th:border-gray-300 prose-th:bg-gray-50 prose-th:p-2 prose-th:text-left prose-th:font-semibold
                        prose-td:border prose-td:border-gray-300 prose-td:p-2
                        prose-a:text-blue-600 prose-a:underline hover:prose-a:text-blue-800" v-html="renderedMarkdown">
            </div>
          </div>

          <div v-if="activeTab === 'json'">
            <VueJsonPretty :data="spec.spec_json" :show-length="true" :show-line="true" :show-icon="true"
              class="max-h-96 overflow-auto rounded-lg" />
          </div>
        </div>
      </div>
    </div>

    <!-- State Information Section -->
    <div class="mb-8" v-if="spec">
      <div class="grid lg:grid-cols-2 gap-8">
        <!-- Current State -->
        <div class="card">
          <h2 class="text-xl font-semibold text-gray-900 mb-4">Current State</h2>
          <div class="flex items-center justify-between p-4 bg-gray-50 rounded-lg">
            <div class="flex items-center">
              <span :class="getStateDotClass(spec?.state)" class="w-4 h-4 rounded-full mr-3"></span>
              <div>
                <p class="font-medium text-gray-900">{{ getStateLabel(spec?.state) }}</p>
                <p class="text-sm text-gray-600">{{ getStateDescription(spec?.state) }}</p>
              </div>
            </div>
            <div class="text-right">
              <p class="text-sm text-gray-500">Last Updated</p>
              <p class="text-sm font-medium text-gray-900">{{ formatDate(spec?.updated_at || spec?.created_at) }}</p>
            </div>
          </div>
        </div>

        <!-- Progress Tracker -->
        <div class="card">
          <h2 class="text-xl font-semibold text-gray-900 mb-4">Progress</h2>
          <div class="space-y-3">
            <div v-for="(state, index) in stateFlow" :key="state" class="flex items-center">
              <div class="flex items-center flex-1">
                <div :class="[
                  'w-3 h-3 rounded-full mr-3',
                  getStateIndex(spec?.state) >= index ? getStateDotClass(state) : 'bg-gray-300'
                ]"></div>
                <span :class="[
                  'text-sm',
                  getStateIndex(spec?.state) >= index ? 'text-gray-900 font-medium' : 'text-gray-500'
                ]">{{ getStateLabel(state) }}</span>
              </div>
              <div v-if="getStateIndex(spec?.state) === index" class="text-xs text-blue-600 font-medium">
                Current
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- State Logs Section -->
    <div class="mb-8" v-if="spec?.state_logs && spec.state_logs.length > 0">
      <div class="card">
        <h2 class="text-xl font-semibold text-gray-900 mb-4">State History</h2>
        <div class="space-y-4">
          <div v-for="log in spec.state_logs" :key="log.id"
            class="flex items-start space-x-3 p-3 bg-gray-50 rounded-lg">
            <span :class="getStateDotClass(log.state_after)" class="w-3 h-3 rounded-full mt-1 flex-shrink-0"></span>
            <div class="flex-1 min-w-0">
              <div class="flex items-center justify-between">
                <p class="text-sm font-medium text-gray-900">{{ getStateLabel(log.state_after) }}</p>
                <p class="text-xs text-gray-500">{{ formatDate(log.created_at) }}</p>
              </div>
              <p v-if="log.detail" class="text-sm text-gray-600 mt-1">{{ log.detail }}</p>
              <p v-if="log.state_before" class="text-xs text-gray-500 mt-1">
                Transitioned from: {{ getStateLabel(log.state_before) }}
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Delete Confirmation Dialog -->
    <ConfirmDialog :show="showDeleteDialog" :loading="deleteLoading" title="Delete Specification"
      :message="`Are you sure you want to delete '${spec?.title}'? This action cannot be undone.`" confirm-text="Delete"
      @confirm="deleteSpec" @cancel="showDeleteDialog = false" />
  </div>
</template>

<script setup>
import VueJsonPretty from 'vue-json-pretty';
import 'vue-json-pretty/lib/styles.css'
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { marked } from 'marked'
import { markedHighlight } from 'marked-highlight'
import hljs from 'highlight.js'
import 'highlight.js/styles/github-dark.css'
import ConfirmDialog from '../components/ConfirmDialog.vue'

const route = useRoute()
const router = useRouter()
const spec = ref(null)
const loading = ref(true)
const error = ref('')
const activeTab = ref('markdown')
const showDeleteDialog = ref(false)
const deleteLoading = ref(false)
const devinTaskLoading = ref(false)

// State flow definition
const stateFlow = ['creating', 'git_initing', 'git_inited', 'code_generating', 'code_generated']

// State helper functions
const getStateClass = (state) => {
  const stateClasses = {
    'creating': 'bg-blue-100 text-blue-800',
    'git_initing': 'bg-yellow-100 text-yellow-800',
    'git_inited': 'bg-indigo-100 text-indigo-800',
    'code_generating': 'bg-purple-100 text-purple-800',
    'code_generated': 'bg-green-100 text-green-800'
  }
  return stateClasses[state] || 'bg-gray-100 text-gray-800'
}

const getStateDotClass = (state) => {
  const dotClasses = {
    'creating': 'bg-blue-500',
    'git_initing': 'bg-yellow-500',
    'git_inited': 'bg-indigo-500',
    'code_generating': 'bg-purple-500',
    'code_generated': 'bg-green-500'
  }
  return dotClasses[state] || 'bg-gray-500'
}

const getStateLabel = (state) => {
  const stateLabels = {
    'creating': 'Creating',
    'git_initing': 'Initializing Git',
    'git_inited': 'Git Ready',
    'code_generating': 'Generating Code',
    'code_generated': 'Code Generated'
  }
  return stateLabels[state] || 'Unknown'
}

const getStateDescription = (state) => {
  const descriptions = {
    'creating': 'Game specification is being created',
    'git_initing': 'Setting up Git repository',
    'git_inited': 'Git repository is ready for code generation',
    'code_generating': 'AI is generating game code',
    'code_generated': 'Game code has been successfully generated'
  }
  return descriptions[state] || 'Unknown state'
}

const getStateIndex = (state) => {
  return stateFlow.indexOf(state)
}

// Configure marked with syntax highlighting
marked.use(markedHighlight({
  langPrefix: 'hljs language-',
  highlight(code, lang) {
    const language = hljs.getLanguage(lang) ? lang : 'plaintext'
    return hljs.highlight(code, { language }).value
  }
}))

marked.setOptions({
  breaks: true,
  gfm: true,
})

// Computed property to render markdown
const renderedMarkdown = computed(() => {
  if (!spec.value?.spec_markdown) return ''
  return marked.parse(spec.value.spec_markdown)
})

const fetchSpec = async () => {
  try {
    loading.value = true
    const response = await fetch(`/api/specs/${route.params.id}`)
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`)
    }
    spec.value = await response.json()
    console.log('Spec data:', spec.value) // Debug log
  } catch (err) {
    console.error('Error fetching spec:', err)
    error.value = 'Failed to load specification. Please try again.'
  } finally {
    loading.value = false
  }
}

const deleteSpec = async () => {
  try {
    deleteLoading.value = true
    const response = await fetch(`/api/specs/${route.params.id}`, {
      method: 'DELETE'
    })

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`)
    }

    // Success - redirect to specs list
    router.push('/specs')
  } catch (err) {
    console.error('Error deleting spec:', err)
    error.value = 'Failed to delete specification. Please try again.'
    showDeleteDialog.value = false
  } finally {
    deleteLoading.value = false
  }
}

const copyToClipboard = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    // You could add a toast notification here
  } catch (err) {
    console.error('Failed to copy to clipboard:', err)
  }
}

const formatDate = (dateString) => {
  if (!dateString) return 'N/A'
  return new Date(dateString).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric'
  })
}

const formatDateTime = (dateString) => {
  if (!dateString) return 'N/A'
  return new Date(dateString).toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit'
  })
}

onMounted(fetchSpec)

const runDevinTask = async () => {
  try {
    devinTaskLoading.value = true
    const response = await fetch(`/api/specs/${route.params.id}/devin-task`, {
      method: 'POST'
    })

    if (!response.ok) {
      const errorData = await response.json()
      throw new Error(errorData.error || `HTTP error! status: ${response.status}`)
    }

    const result = await response.json()

    // Show success message with session URL
    const message = `Devin task created successfully for "${result.game_title}"!\n\nSession URL: ${result.session_url}\nRepository: ${result.repository}`
    alert(message)

    // Refresh the spec data to show the new session information
    await fetchSpec()

  } catch (err) {
    console.error('Error creating Devin task:', err)
    alert(`Failed to create Devin task: ${err.message}`)
  } finally {
    devinTaskLoading.value = false
  }
}
</script>
