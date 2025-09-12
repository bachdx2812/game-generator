<template>
  <div class="min-h-screen bg-gray-50">
    <!-- Header Section -->
    <div class="bg-white shadow-sm border-b">
      <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        <div class="flex justify-between items-center">
          <div>
            <h1 class="text-3xl font-bold text-gray-900">Game Specifications</h1>
            <p class="mt-2 text-gray-600">Manage and view your game specifications</p>
          </div>
          <router-link to="/"
            class="inline-flex items-center px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors duration-200 font-medium">
            <svg class="w-5 h-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"></path>
            </svg>
            Generate New Spec
          </router-link>
        </div>
      </div>
    </div>

    <!-- Stats Section -->
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
      <div class="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8">
        <div class="bg-white rounded-xl p-6 shadow-sm border">
          <div class="flex items-center">
            <div class="p-3 bg-blue-100 rounded-lg">
              <svg class="w-6 h-6 text-blue-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                  d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z">
                </path>
              </svg>
            </div>
            <div class="ml-4">
              <p class="text-sm font-medium text-gray-600">Total Specs</p>
              <p class="text-2xl font-bold text-gray-900">{{ specs.length }}</p>
            </div>
          </div>
        </div>

        <div class="bg-white rounded-xl p-6 shadow-sm border">
          <div class="flex items-center">
            <div class="p-3 bg-green-100 rounded-lg">
              <svg class="w-6 h-6 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>
              </svg>
            </div>
            <div class="ml-4">
              <p class="text-sm font-medium text-gray-600">Completed</p>
              <p class="text-2xl font-bold text-gray-900">{{ completedSpecs }}</p>
            </div>
          </div>
        </div>

        <div class="bg-white rounded-xl p-6 shadow-sm border">
          <div class="flex items-center">
            <div class="p-3 bg-yellow-100 rounded-lg">
              <svg class="w-6 h-6 text-yellow-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                  d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"></path>
              </svg>
            </div>
            <div class="ml-4">
              <p class="text-sm font-medium text-gray-600">In Progress</p>
              <p class="text-2xl font-bold text-gray-900">{{ inProgressSpecs }}</p>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Main Content -->
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 pb-12">
      <!-- Loading State -->
      <div v-if="loading" class="flex justify-center items-center py-12">
        <div class="text-center">
          <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600 mx-auto mb-4"></div>
          <p class="text-gray-600">Loading specifications...</p>
        </div>
      </div>

      <!-- Error State -->
      <div v-else-if="error" class="bg-red-50 border border-red-200 rounded-lg p-6 text-center">
        <svg class="w-12 h-12 text-red-400 mx-auto mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
            d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path>
        </svg>
        <h3 class="text-lg font-medium text-red-800 mb-2">Error Loading Specifications</h3>
        <p class="text-red-600 mb-4">{{ error }}</p>
        <button @click="fetchSpecs"
          class="px-4 py-2 bg-red-600 text-white rounded-lg hover:bg-red-700 transition-colors duration-200">
          Try Again
        </button>
      </div>

      <!-- Empty State -->
      <div v-else-if="specs.length === 0" class="text-center py-12">
        <div class="bg-white rounded-xl p-12 shadow-sm border">
          <svg class="w-16 h-16 text-gray-400 mx-auto mb-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
              d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z">
            </path>
          </svg>
          <h3 class="text-xl font-medium text-gray-900 mb-2">No specifications yet</h3>
          <p class="text-gray-500 mb-6">Get started by creating your first game specification</p>
          <router-link to="/"
            class="inline-flex items-center px-6 py-3 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors duration-200 font-medium">
            <svg class="w-5 h-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"></path>
            </svg>
            Generate First Spec
          </router-link>
        </div>
      </div>

      <!-- Specs Grid -->
      <div v-else class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        <div v-for="spec in specs" :key="spec.id"
          class="bg-white rounded-xl shadow-sm border hover:shadow-md transition-all duration-200 hover:-translate-y-1">
          <!-- Card Header -->
          <div class="px-6 py-4 border-b border-gray-200">
            <div class="flex items-start justify-between">
              <div class="flex-1">
                <h3 class="text-lg font-semibold text-gray-900 mb-1">{{ spec.title }}</h3>
                <p class="text-sm text-gray-600 line-clamp-2">{{ spec.brief }}</p>
              </div>
              <div class="flex flex-col items-end space-y-1">
                <span class="text-xs text-gray-500">{{ formatDate(spec.created_at) }}</span>
                <!-- Devin Session Indicator -->
                <div v-if="spec.devin_session_id"
                  class="flex items-center text-xs text-green-600 bg-green-100 px-2 py-1 rounded-full">
                  <svg class="w-3 h-3 mr-1" fill="currentColor" viewBox="0 0 20 20">
                    <path fill-rule="evenodd"
                      d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                      clip-rule="evenodd"></path>
                  </svg>
                  Devin Active
                </div>
              </div>
            </div>
          </div>

          <!-- Code Job Status -->
          <div class="px-6 pb-4">
            <CodeJobStatus :spec-id="spec.id" />
          </div>

          <!-- Card Actions -->
          <div class="px-6 pb-6">
            <div class="space-y-2">
              <router-link :to="`/specs/${spec.id}`"
                class="w-full inline-flex justify-center items-center px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors duration-200 font-medium">
                <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                    d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path>
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                    d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z">
                  </path>
                </svg>
                View Details
              </router-link>

              <!-- Show Devin session link if available, otherwise show create button -->
              <a v-if="spec.devin_session_id" :href="spec.devin_session_url" target="_blank" rel="noopener noreferrer"
                class="w-full inline-flex justify-center items-center px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 transition-colors duration-200 font-medium">
                <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                    d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"></path>
                </svg>
                Open in Devin
              </a>
              <button v-else @click="runDevinTask(spec.id)" :disabled="devinTaskLoading[spec.id]"
                class="w-full inline-flex justify-center items-center px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 disabled:bg-gray-400 disabled:cursor-not-allowed transition-colors duration-200 font-medium">
                <svg v-if="!devinTaskLoading[spec.id]" class="w-4 h-4 mr-2" fill="none" stroke="currentColor"
                  viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z">
                  </path>
                </svg>
                <svg v-else class="animate-spin w-4 h-4 mr-2" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor"
                    d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z">
                  </path>
                </svg>
                {{ devinTaskLoading[spec.id] ? 'Creating Task...' : 'Run Devin Task' }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Delete Confirmation Dialog -->
    <ConfirmDialog :show="showDeleteDialog" :loading="deleteLoading" title="Delete Specification"
      :message="`Are you sure you want to delete '${specToDelete?.title}'? This action cannot be undone and will remove the spec from both the database and vector database.`"
      confirm-text="Delete" @confirm="deleteSpec" @cancel="cancelDelete" />
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import CodeJobStatus from '../components/CodeJobStatus.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'

const specs = ref([])
const loading = ref(true)
const error = ref('')
const showDeleteDialog = ref(false)
const deleteLoading = ref(false)
const specToDelete = ref(null)

const completedSpecs = computed(() => {
  return specs.value.filter(spec => spec.code_job_status === 'completed').length
})

const inProgressSpecs = computed(() => {
  return specs.value.filter(spec =>
    spec.code_job_status === 'pending' || spec.code_job_status === 'running'
  ).length
})

const fetchSpecs = async () => {
  try {
    loading.value = true
    error.value = ''
    const response = await fetch('/api/specs')
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`)
    }
    const data = await response.json()
    console.log('API Response:', data)
    specs.value = data
  } catch (err) {
    console.error('Error fetching specs:', err)
    error.value = 'Failed to load specifications. Please try again.'
  } finally {
    loading.value = false
  }
}

const confirmDelete = (spec) => {
  specToDelete.value = spec
  showDeleteDialog.value = true
}

const cancelDelete = () => {
  showDeleteDialog.value = false
  specToDelete.value = null
}

const deleteSpec = async () => {
  if (!specToDelete.value) return

  try {
    deleteLoading.value = true
    const response = await fetch(`/api/specs/${specToDelete.value.id}`, {
      method: 'DELETE'
    })

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`)
    }

    specs.value = specs.value.filter(spec => spec.id !== specToDelete.value.id)
    showDeleteDialog.value = false
    specToDelete.value = null
  } catch (err) {
    console.error('Error deleting spec:', err)
    error.value = 'Failed to delete specification. Please try again.'
  } finally {
    deleteLoading.value = false
  }
}

const formatDate = (dateString) => {
  return new Date(dateString).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric'
  })
}

onMounted(fetchSpecs)

const devinTaskLoading = ref({})
const devinTaskStatus = ref({})

const runDevinTask = async (specId) => {
  try {
    devinTaskLoading.value[specId] = true
    const response = await fetch(`/api/specs/${specId}/devin-task`, {
      method: 'POST'
    })

    if (!response.ok) {
      const errorData = await response.json()
      throw new Error(errorData.error || `HTTP error! status: ${response.status}`)
    }

    const result = await response.json()
    devinTaskStatus.value[specId] = 'success'

    // Show success message with session URL
    const message = `Devin task created successfully for "${result.game_title}"!\n\nSession URL: ${result.session_url}\nRepository: ${result.repository}`
    alert(message)

    // Refresh the specs list to show updated session information
    await fetchSpecs()

  } catch (err) {
    console.error('Error creating Devin task:', err)
    devinTaskStatus.value[specId] = 'error'
    const errorMessage = err.message || 'Failed to create Devin task. Please try again.'
    alert(`Failed to create Devin task: ${errorMessage}`)
  } finally {
    devinTaskLoading.value[specId] = false
  }
}
</script>
