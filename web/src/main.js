import { createApp } from 'vue'
import router from './router'
import Layout from './components/Layout.vue'
import './style.css'

const app = createApp(Layout)
app.use(router)
app.mount('#app')
