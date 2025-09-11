<template>
  <div class="container mx-auto px-4 py-6">
    <div class="flex flex-col sm:flex-row sm:items-center justify-between mb-8 gap-4">
      <div>
        <h1 class="text-3xl font-bold text-gray-900">Game Specifications</h1>
        <p class="text-gray-600 mt-1">Browse all generated game specs</p>
      </div>
      <router-link to="/" class="btn-primary whitespace-nowrap">
        Generate New Spec
      </router-link>
    </div>

    <!-- Loading State -->
    <div v-if="loading" class="flex justify-center py-16">
      <div class="text-center">
        <svg class="animate-spin h-12 w-12 text-primary-500 mx-auto mb-4" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z">
          </path>
        </svg>
        <p class="text-gray-600">Loading specifications...</p>
      </div>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="bg-red-50 border border-red-200 text-red-700 px-6 py-4 rounded-lg">
      <div class="flex items-center">
        <svg class="h-5 w-5 mr-2" fill="currentColor" viewBox="0 0 20 20">
          <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z" clip-rule="evenodd"></path>
        </svg>
        {{ error }}
      </div>
    </div>

    <!-- Specs Grid -->
    <div v-else-if="specs && specs.length > 0" class="grid gap-6 sm:grid-cols-1 md:grid-cols-2 xl:grid-cols-3">
      <div v-for="spec in specs" :key="spec.id" class="card hover:shadow-lg transition-all duration-200 hover:-translate-y-1">
        <div class="cursor-pointer" @click="$router.push(`/specs/${spec.id}`)">
          <h3 class="text-xl font-semibold text-gray-900 mb-2 line-clamp-2">{{ spec.title }}</h3>
          <p class="text-sm text-gray-500 mb-4">
            Created {{ formatDate(spec.created_at) }}
          </p>
        </div>

        <div class="space-y-4 pt-4 border-t border-gray-200">
          <!-- Code Job Status -->
          <div class="w-full">
            <CodeJobStatus :spec-id="spec.id" />
          </div>

          <!-- Action Buttons -->
          <div class="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-3">
            <router-link :to="`/specs/${spec.id}`" class="btn-secondary text-sm flex-1 sm:flex-none text-center">
              View Details
            </router-link>
            <button @click="confirmDelete(spec)" class="btn-outline text-sm text-red-600 border-red-300 hover:bg-red-50 hover:border-red-400 flex-1 sm:flex-none">
              Delete
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Empty State -->
    <div v-else class="text-center py-16">
      <div class="max-w-md mx-auto">
        <svg class="mx-auto h-16 w-16 text-gray-400 mb-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
            d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
        </svg>
        <h3 class="text-lg font-medium text-gray-900 mb-2">No specifications</h3>
        <p class="text-gray-500 mb-6">Get started by generating your first game specification.</p>
        <router-link to="/" class="btn-primary">
          Generate First Spec
        </router-link>
      </div>
    </div>

    <!-- Delete Confirmation Dialog -->
    <ConfirmDialog :show="showDeleteDialog" :loading="deleteLoading" title="Delete Specification"
      :message="`Are you sure you want to delete '${specToDelete?.title}'? This action cannot be undone and will remove the spec from both the database and vector database.`"
      confirm-text="Delete" @confirm="deleteSpec" @cancel="cancelDelete" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import CodeJobStatus from '../components/CodeJobStatus.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'

const specs = ref<any[]>([])
const loading = ref(true)
const error = ref('')
const showDeleteDialog = ref(false)
const deleteLoading = ref(false)
const specToDelete = ref<any>(null)

const fetchSpecs = async () => {
  try {
    loading.value = true
    const response = await fetch('/api/specs')
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`)
    }
    const data = await response.json()
    console.log('API Response:', data) // Add this debug line
    specs.value = data
  } catch (err) {
    console.error('Error fetching specs:', err)
    error.value = 'Failed to load specifications. Please try again.'
  } finally {
    loading.value = false
  }
}

const confirmDelete = (spec: any) => {
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

    // Remove the deleted spec from the list
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

const formatDate = (dateString: string) => {
  return new Date(dateString).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric'
  })
}

onMounted(fetchSpecs)
</script>
