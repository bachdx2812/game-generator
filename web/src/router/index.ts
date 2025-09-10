import { createRouter, createWebHistory } from 'vue-router'
import Home from '@/views/Home.vue'
import SpecList from '@/views/SpecList.vue'
import SpecDetail from '@/views/SpecDetail.vue'

const routes = [
  {
    path: '/',
    name: 'Home',
    component: Home
  },
  {
    path: '/specs',
    name: 'SpecList',
    component: SpecList
  },
  {
    path: '/specs/:id',
    name: 'SpecDetail',
    component: SpecDetail,
    props: true
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

export default router
