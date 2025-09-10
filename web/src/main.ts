import { createApp, ref } from 'vue'

const App = {
  setup() {
    const brief = ref('arcade 60s mobile')
    const job = ref<any>(null)
    const duplicateList = ref<any[]>([])
    const spec = ref<any>(null)
    const loading = ref(false)
    const message = ref('')

    const createJob = async () => {
      loading.value = true
      message.value = ''
      duplicateList.value = []
      spec.value = null

      const res = await fetch('/api/spec-jobs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ brief: brief.value })
      })
      const data = await res.json()
      loading.value = false
      job.value = data

      if (data.status === 'DUPLICATE') {
        duplicateList.value = data.duplicate_list || []
      } else if (data.status === 'COMPLETED') {
        const sres = await fetch(`/api/specs/${data.result_spec_id}`)
        if (sres.ok) spec.value = await sres.json()
      } else {
        message.value = 'Unexpected response, check backend logs.'
      }
    }

    return { brief, createJob, job, duplicateList, spec, loading, message }
  },
  template: `
  <div style="max-width:800px;margin:40px auto;font-family:system-ui, -apple-system, Segoe UI, Roboto;">
    <h1>Spec Planner</h1>
    <div style="display:flex;gap:8px;">
      <input v-model="brief" placeholder="Enter brief..." style="flex:1;padding:8px;border:1px solid #ccc;border-radius:8px;">
      <button @click="createJob" :disabled="loading" style="padding:8px 12px;border-radius:8px;">Generate</button>
    </div>
    <p v-if="message" style="color:#c00">{{ message }}</p>
    <div v-if="loading" style="margin-top:16px;">Generating...</div>

    <div v-if="job && job.status==='DUPLICATE'" style="margin-top:20px;">
      <h3>Similar specs</h3>
      <ul>
        <li v-for="it in duplicateList" :key="it.id">
          <strong>{{ it.title }}</strong> ({{ it.id }}) score ~ {{ it.score?.toFixed?.(2) ?? 'n/a' }}
        </li>
      </ul>
    </div>

    <div v-if="spec" style="margin-top:20px;">
      <h2>{{ spec.title }}</h2>
      <p><em>Brief:</em> {{ spec.brief }}</p>
      <details>
        <summary>Spec JSON</summary>
        <pre>{{ JSON.stringify(spec.spec_json, null, 2) }}</pre>
      </details>
      <details>
        <summary>Spec Markdown</summary>
        <pre style="white-space:pre-wrap">{{ spec.spec_markdown }}</pre>
      </details>
    </div>
  </div>
  `
}

createApp(App).mount('#app')
