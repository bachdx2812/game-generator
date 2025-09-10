<template>
  <div>
    <div class="flex items-center justify-between mb-8">
      <div>
        <h1 class="text-3xl font-bold text-gray-900">Game Specifications</h1>
        <p class="text-gray-600 mt-1">Browse all generated game specs</p>
      </div>
      <router-link to="/" class="btn-primary">
        Generate New Spec
      </router-link>
    </div>

    <!-- Loading State -->
    <div v-if="loading" class="flex justify-center py-12">
      <svg class="animate-spin h-8 w-8 text-primary-500" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
      </svg>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded-lg">
      {{ error }}
    </div>

    <!-- Specs Grid -->
    <div v-else-if="specs.length > 0" class="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
      <div
        v-for="spec in specs"
        :key="spec.id"
        class="card hover:shadow-lg transition-shadow duration-200 cursor-pointer"
        @click="$router.push(`/specs/${spec.id}`)"
      >
        <h3 class="text-lg font-semibold text-gray-900 mb-2">{{ spec.title }}</h3>
        <p class="text-sm text-gray-500 mb-4">
          Created {{ formatDate(spec.created_at) }}
        </p>
        <div class="flex justify-between items-center">
          <span class="text-xs bg-primary-100 text-primary-800 px-2 py-1 rounded-full">
            Game Spec
          </span>
          <svg class="w-5 h-5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"></path>
          </svg>
        </div>
      </div>
    </div>

    <!-- Empty State -->
    <div v-else class="text-center py-12">
      <svg class="mx-auto h-12 w-12 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"></path>
      </svg>
      <h3 class="mt-2 text-sm font-medium text-gray-900">No specs found</h3>
      <p class="mt-1 text-sm text-gray-500">Get started by generating your first game spec.</p>
      <div class="mt-6">
        <router-link to="/" class="btn-primary">
          Generate First Spec
        </router-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'

const specs = ref<any[]>([])
const loading = ref(true)
const error = ref('')

const fetchSpecs = async () => {
  try {
    loading.value = true
    const response = await fetch('/api/specs')
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`)
    }
    specs.value = await response.json()
  } catch (err) {
    console.error('Error fetching specs:', err)
    error.value = 'Failed to load specifications. Please try again.'
  } finally {
    loading.value = false
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
