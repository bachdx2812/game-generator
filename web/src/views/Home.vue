<template>
  <div class="max-w-4xl mx-auto">
    <div class="text-center mb-8">
      <h1 class="text-3xl font-bold text-gray-900 mb-2">Game Spec Generator</h1>
      <p class="text-gray-600">Generate detailed game specifications with AI</p>
    </div>

    <!-- Generator Form -->
    <div class="card mb-8">
      <div class="flex flex-col sm:flex-row gap-4">
        <input v-model="brief" placeholder="Enter game brief (e.g., 'arcade 60s mobile puzzle game')"
          class="input-field flex-1" @keyup.enter="createJob">
        <button @click="createJob" :disabled="loading || !brief.trim()" class="btn-primary whitespace-nowrap">
          <span v-if="loading" class="flex items-center">
            <svg class="animate-spin -ml-1 mr-2 h-4 w-4 text-white" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z">
              </path>
            </svg>
            Generating...
          </span>
          <span v-else>Generate Spec</span>
        </button>
      </div>
    </div>

    <!-- Error Message -->
    <div v-if="message" class="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded-lg mb-6">
      {{ message }}
    </div>

    <!-- Duplicate Results -->
    <div v-if="job && job.status === 'DUPLICATE'" class="card mb-8">
      <h3 class="text-lg font-semibold text-gray-900 mb-4">Similar Existing Specs</h3>
      <div class="space-y-3">
        <div v-for="item in duplicateList" :key="item.id"
          class="flex items-center justify-between p-4 bg-gray-50 rounded-lg hover:bg-gray-100 transition-colors">
          <div>
            <h4 class="font-medium text-gray-900">{{ item.title }}</h4>
            <p class="text-sm text-gray-600">Similarity: {{ (item.score * 100).toFixed(1) }}%</p>
          </div>
          <router-link :to="`/specs/${item.id}`" class="btn-secondary text-sm">
            View Details
          </router-link>
        </div>
      </div>
    </div>

    <!-- Generated Spec -->
    <div v-if="spec" class="card">
      <div class="flex items-center justify-between mb-6">
        <h2 class="text-2xl font-bold text-gray-900">{{ spec.title }}</h2>
        <router-link :to="`/specs/${spec.id}`" class="btn-secondary">
          View Full Details
        </router-link>
      </div>

      <div class="mb-4">
        <span class="text-sm font-medium text-gray-500">Brief:</span>
        <p class="text-gray-700 mt-1">{{ spec.brief }}</p>
      </div>

      <!-- Quick Preview -->
      <div class="grid md:grid-cols-2 gap-6">
        <div>
          <h3 class="font-semibold text-gray-900 mb-2">Game Details</h3>
          <div class="space-y-2 text-sm">
            <div><span class="font-medium">Genre:</span> {{ spec.spec_json?.genre || 'N/A' }}</div>
            <div><span class="font-medium">Duration:</span> {{ spec.spec_json?.duration_sec || 'N/A' }}s</div>
            <div><span class="font-medium">Platform:</span> {{ spec.spec_json?.platform?.join(', ') || 'N/A' }}</div>
          </div>
        </div>
        <div>
          <h3 class="font-semibold text-gray-900 mb-2">Game Modes</h3>
          <div class="space-y-1 text-sm">
            <div v-for="mode in spec.spec_json?.game_modes || []" :key="mode.mode" class="flex items-center">
              <span class="w-2 h-2 bg-primary-500 rounded-full mr-2"></span>
              {{ mode.mode?.replace('_', ' ') || 'Unknown' }}
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()
const brief = ref('')
const job = ref<any>(null)
const duplicateList = ref<any[]>([])
const spec = ref<any>(null)
const loading = ref(false)
const message = ref('')

const createJob = async () => {
  if (!brief.value.trim()) return

  loading.value = true
  message.value = ''
  duplicateList.value = []
  spec.value = null

  try {
    const res = await fetch('/api/spec-jobs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ brief: brief.value })
    })

    if (!res.ok) {
      throw new Error(`HTTP error! status: ${res.status}`)
    }

    const data = await res.json()
    job.value = data

    if (data.status === 'DUPLICATE') {
      duplicateList.value = data.duplicate_list || []
    } else if (data.status === 'COMPLETED') {
      const sres = await fetch(`/api/specs/${data.result_spec_id}`)
      if (sres.ok) {
        spec.value = await sres.json()
      } else {
        message.value = 'Failed to fetch generated spec details.'
      }
    } else {
      message.value = 'Unexpected response, check backend logs.'
    }
  } catch (error) {
    console.error('Error creating job:', error)
    message.value = 'Failed to generate spec. Please try again.'
  } finally {
    loading.value = false
  }
}
</script>
